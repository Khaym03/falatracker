package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Page wraps a rod page so that every action:
//   - is bound to the caller's context (cancellation stops it),
//   - is limited by a timeout (it never hangs forever),
//   - returns an error instead of panicking like rod's Must* helpers.
type Page struct {
	rod     *rod.Page
	timeout time.Duration
}

// NewPage wraps p. timeout bounds each individual action.
func NewPage(p *rod.Page, timeout time.Duration) *Page {
	return &Page{rod: p, timeout: timeout}
}

// bind returns the underlying page tied to ctx plus the action timeout.
func (p *Page) bind(ctx context.Context) (*rod.Page, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	return p.rod.Context(ctx), cancel
}

// URL returns the current page URL.
func (p *Page) URL(ctx context.Context) (string, error) {
	rp, cancel := p.bind(ctx)
	defer cancel()

	info, err := rp.Info()
	if err != nil {
		return "", fmt.Errorf("get page info: %w", err)
	}
	return info.URL, nil
}

// Navigate opens url and waits for the page to load.
func (p *Page) Navigate(ctx context.Context, url string) error {
	return p.withNavigation(ctx, func(rp *rod.Page) error {
		if err := rp.Navigate(url); err != nil {
			return fmt.Errorf("navigate to %s: %w", url, err)
		}
		if err := rp.WaitLoad(); err != nil {
			return fmt.Errorf("wait load of %s: %w", url, err)
		}
		return nil
	})
}

// Click waits for the element matching selector to be visible and clicks it.
func (p *Page) Click(ctx context.Context, selector string) error {
	rp, cancel := p.bind(ctx)
	defer cancel()
	return clickSelector(rp, selector)
}

// ClickAndWait clicks the element matching selector and waits for the
// navigation it triggers.
func (p *Page) ClickAndWait(ctx context.Context, selector string) error {
	return p.withNavigation(ctx, func(rp *rod.Page) error {
		return clickSelector(rp, selector)
	})
}

// ClickNth clicks the element at index (0-based) among those matching selector.
func (p *Page) ClickNth(ctx context.Context, selector string, index int) error {
	rp, cancel := p.bind(ctx)
	defer cancel()

	if _, err := rp.Element(selector); err != nil {
		return fmt.Errorf("find %q: %w", selector, err)
	}
	els, err := rp.Elements(selector)
	if err != nil {
		return fmt.Errorf("find %q: %w", selector, err)
	}
	if index >= len(els) {
		return fmt.Errorf("find %q: want element %d, found %d", selector, index, len(els))
	}
	return clickElement(els[index], selector)
}

// ClickText clicks the first element whose text matches text.
func (p *Page) ClickText(ctx context.Context, text string) error {
	rp, cancel := p.bind(ctx)
	defer cancel()

	// Search enables the DOM domain and disables it again on Release. If the
	// action times out, that disable runs with an expired context and leaves
	// the page in a bad state for the next attempt. Keeping the domain
	// enabled makes Search skip the toggle.
	if err := (proto.DOMEnable{}).Call(rp); err != nil {
		return fmt.Errorf("enable DOM domain: %w", err)
	}

	sr, err := rp.Search(text)
	if err != nil {
		return fmt.Errorf("search text %q: %w", text, err)
	}
	defer sr.Release()

	return clickElement(sr.First, text)
}

// Input types text into the element matching selector.
func (p *Page) Input(ctx context.Context, selector, text string) error {
	rp, cancel := p.bind(ctx)
	defer cancel()

	el, err := rp.Element(selector)
	if err != nil {
		return fmt.Errorf("find %q: %w", selector, err)
	}
	if err := el.Input(text); err != nil {
		return fmt.Errorf("input into %q: %w", selector, err)
	}
	return nil
}

// Exists reports whether an element matching selector appears within timeout.
// It only returns an error if ctx is done or the browser fails.
func (p *Page) Exists(ctx context.Context, selector string, timeout time.Duration) (bool, error) {
	return p.exists(ctx, timeout, func(rp *rod.Page) error {
		_, err := rp.Element(selector)
		return err
	})
}

// ExistsXPath is like Exists but takes an XPath expression.
func (p *Page) ExistsXPath(ctx context.Context, xpath string, timeout time.Duration) (bool, error) {
	return p.exists(ctx, timeout, func(rp *rod.Page) error {
		_, err := rp.ElementX(xpath)
		return err
	})
}

// RowTexts waits for at least one element matching rowSelector and returns
// the trimmed text of each <td> of every matching row.
func (p *Page) RowTexts(ctx context.Context, rowSelector string) ([][]string, error) {
	rp, cancel := p.bind(ctx)
	defer cancel()

	rows, err := waitRows(rp, rowSelector)
	if err != nil {
		return nil, err
	}

	out := make([][]string, 0, len(rows))
	for i, row := range rows {
		texts, err := cellTexts(row)
		if err != nil {
			return nil, fmt.Errorf("read row %d: %w", i+1, err)
		}
		out = append(out, texts)
	}
	return out, nil
}

// ClickInRowAndWait finds the first row (matching rowSelector) whose cell at
// column equals value, clicks the element matching target inside that row and
// waits for the navigation it triggers.
func (p *Page) ClickInRowAndWait(ctx context.Context, rowSelector string, column int, value, target string) error {
	return p.withNavigation(ctx, func(rp *rod.Page) error {
		row, err := findRow(rp, rowSelector, column, value)
		if err != nil {
			return err
		}
		el, err := row.Element(target)
		if err != nil {
			return fmt.Errorf("find %q in row %q: %w", target, value, err)
		}
		return clickElement(el, target)
	})
}

// Download runs action, which must start exactly one file download, and
// waits until the browser has completely saved the file in dir. The file
// keeps the name suggested by the site (replacing any file with that name)
// and its path is returned.
func (p *Page) Download(ctx context.Context, dir string, timeout time.Duration, action func() error) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("download dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create download dir: %w", err)
	}

	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Chrome saves the file as <dir>/<GUID> and reports progress events.
	wait := p.rod.Browser().Context(tctx).WaitDownload(abs)
	if err := action(); err != nil {
		return "", err
	}
	info := wait()
	// On timeout rod still returns the started download, so check the context.
	if err := tctx.Err(); err != nil {
		return "", fmt.Errorf("download did not complete: %w", err)
	}
	if info == nil {
		return "", errors.New("download did not start")
	}

	name := filepath.Base(info.SuggestedFilename)
	if name == "." || name == string(filepath.Separator) {
		name = info.GUID
	}
	dst := filepath.Join(abs, name)
	if err := os.Rename(filepath.Join(abs, info.GUID), dst); err != nil {
		return "", fmt.Errorf("rename downloaded file: %w", err)
	}
	return dst, nil
}

// withNavigation runs action and then waits for the page to become network
// idle. Not reaching idle before the timeout is tolerated (some pages keep
// polling), but cancellation of ctx is reported.
func (p *Page) withNavigation(ctx context.Context, action func(*rod.Page) error) error {
	rp, cancel := p.bind(ctx)
	defer cancel()

	wait := rp.WaitNavigation(proto.PageLifecycleEventNameNetworkAlmostIdle)
	if err := action(rp); err != nil {
		return err
	}
	wait()
	return ctx.Err()
}

func (p *Page) exists(ctx context.Context, timeout time.Duration, find func(*rod.Page) error) (bool, error) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := find(p.rod.Context(tctx))
	switch {
	case err == nil:
		return true, nil
	case ctx.Err() != nil:
		return false, ctx.Err()
	case errors.Is(err, context.DeadlineExceeded):
		return false, nil
	default:
		return false, err
	}
}

func clickSelector(rp *rod.Page, selector string) error {
	el, err := rp.Element(selector)
	if err != nil {
		return fmt.Errorf("find %q: %w", selector, err)
	}
	return clickElement(el, selector)
}

func clickElement(el *rod.Element, name string) error {
	if err := el.WaitVisible(); err != nil {
		return fmt.Errorf("wait %q visible: %w", name, err)
	}
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("click %q: %w", name, err)
	}
	return nil
}

func waitRows(rp *rod.Page, rowSelector string) (rod.Elements, error) {
	if _, err := rp.Element(rowSelector); err != nil {
		return nil, fmt.Errorf("wait for rows %q: %w", rowSelector, err)
	}
	rows, err := rp.Elements(rowSelector)
	if err != nil {
		return nil, fmt.Errorf("find rows %q: %w", rowSelector, err)
	}
	return rows, nil
}

func findRow(rp *rod.Page, rowSelector string, column int, value string) (*rod.Element, error) {
	rows, err := waitRows(rp, rowSelector)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		texts, err := cellTexts(row)
		if err != nil {
			return nil, err
		}
		if column < len(texts) && texts[column] == value {
			return row, nil
		}
	}
	return nil, fmt.Errorf("no row with %q in column %d", value, column)
}

func cellTexts(row *rod.Element) ([]string, error) {
	cells, err := row.Elements("td")
	if err != nil {
		return nil, fmt.Errorf("find cells: %w", err)
	}
	texts := make([]string, len(cells))
	for i, c := range cells {
		t, err := c.Text()
		if err != nil {
			return nil, fmt.Errorf("read cell %d: %w", i+1, err)
		}
		texts[i] = strings.TrimSpace(t)
	}
	return texts, nil
}
