// Package portal automates the Falabella Business Center website: login,
// listing purchase orders and downloading their Excel reports. Everything
// site-specific (URLs, selectors, flows) lives here.
package portal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/Khaym03/falatracker/internal/browser"
	"github.com/Khaym03/falatracker/internal/order"
)

const (
	ordersURL = `https://fbusinesscenter.com/purchase-orders`
	loginURL  = `https://access-key-corp.falabella.tech/auth/realms/esti/protocol/openid-connect/auth?client_id=portal&redirect_uri=https%3A%2F%2Ffbusinesscenter.com%2F&response_type=code&scope=openid&state=1739716db8fc4511b91fb1bf240805d7&code_challenge=nHB46AWINHfK5cOfiT5PyRQ7bDK5gZ70NiRMVUxCvjo&code_challenge_method=S256&fpkt=true`
)

// Selectors of the portal markup.
const (
	usernameInput  = `input[id="username"]`
	passwordInput  = `input[id="password"]`
	loginButton    = `form button`
	loggedInMarker = `//input[@value='Falabella']`
	postLoginLink  = `a[href="/login"]`

	ordersCard     = `a[href="/purchase-orders"]`
	historyTab     = `div[id="single-spa-application:purchaseOrders"] div[aria-label="po tabs"][role="tablist"] button:nth-of-type(2)`
	orderRows      = `table[aria-labelledby="tableTitle"] tbody tr`
	rowOpenButton  = `button`
	downloadButton = `button[id="donwload-button"]` // "donwload" is the portal's own spelling
	excelOption    = `Excel`
)

const (
	actionTimeout  = 20 * time.Second // limit for a single browser action
	probeTimeout   = 20 * time.Second // how long to look for optional elements
	downloadSettle = 10 * time.Second // time given to the browser to finish downloads
	clickAttempts  = 2
	retryDelay     = time.Second

	orderNumberColumn = 1
	reportsPerOrder   = 2
)

// Fragments of the report file names, e.g. oc-1000001-products-details-07-11-2026.xlsx.
const (
	productsReport     = "products-details"
	distributionReport = "distributions-details"
)

// Credential holds the portal login.
type Credential struct {
	User     string
	Password string
}

// Validate checks that both fields are set.
func (c Credential) Validate() error {
	var missing []string
	if c.User == "" {
		missing = append(missing, "user")
	}
	if c.Password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s", strings.Join(missing, " and "))
	}
	return nil
}

// Client opens portal sessions on a running browser.
type Client struct {
	browser     *rod.Browser
	cred        Credential
	downloadDir string
	log         *slog.Logger
}

// New returns a Client that uses b. Reports are saved in
// <downloadDir>/<order number>/.
func New(b *rod.Browser, cred Credential, downloadDir string, log *slog.Logger) *Client {
	return &Client{browser: b, cred: cred, downloadDir: downloadDir, log: log}
}

// Open closes the tabs left by previous cycles and returns a session on a new tab.
func (c *Client) Open(ctx context.Context) (*Session, error) {
	b := c.browser.Context(ctx)

	pages, err := b.Pages()
	if err != nil {
		return nil, fmt.Errorf("list tabs: %w", err)
	}
	// Keep the first tab so the browser window stays open.
	if len(pages) > 1 {
		for _, p := range pages[1:] {
			if err := p.Close(); err != nil {
				c.log.Warn("failed to close stale tab", "error", err)
			}
		}
	}

	p, err := b.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, fmt.Errorf("open tab: %w", err)
	}
	return newSession(browser.NewPage(p, actionTimeout), c.cred, c.downloadDir, c.log), nil
}

// Session runs the portal flows on a single tab.
type Session struct {
	page        *browser.Page
	cred        Credential
	downloadDir string
	log         *slog.Logger
}

func newSession(page *browser.Page, cred Credential, downloadDir string, log *slog.Logger) *Session {
	return &Session{page: page, cred: cred, downloadDir: downloadDir, log: log}
}

// Login signs in, unless the browser profile already has a valid session.
func (s *Session) Login(ctx context.Context) error {
	if err := s.page.Navigate(ctx, ordersURL); err != nil {
		return fmt.Errorf("open purchase orders: %w", err)
	}

	loggedIn, err := s.page.ExistsXPath(ctx, loggedInMarker, probeTimeout)
	if err != nil {
		return fmt.Errorf("check login state: %w", err)
	}
	if loggedIn {
		s.log.Debug("already logged in")
		return nil
	}

	s.log.Info("logging in", "user", s.cred.User)
	if err := s.page.Navigate(ctx, loginURL); err != nil {
		return fmt.Errorf("open login page: %w", err)
	}
	if err := s.page.Input(ctx, usernameInput, s.cred.User); err != nil {
		return fmt.Errorf("fill user: %w", err)
	}
	if err := s.page.Input(ctx, passwordInput, s.cred.Password); err != nil {
		return fmt.Errorf("fill password: %w", err)
	}
	// Wait for the whole redirect chain of the login.
	if err := s.page.ClickAndWait(ctx, loginButton); err != nil {
		return fmt.Errorf("submit login: %w", err)
	}

	// The portal may land on a page with a login link that must be followed.
	hasLink, err := s.page.Exists(ctx, postLoginLink, probeTimeout)
	if err != nil {
		return fmt.Errorf("check post-login link: %w", err)
	}
	if hasLink {
		if err := s.page.ClickAndWait(ctx, postLoginLink); err != nil {
			return fmt.Errorf("follow post-login link: %w", err)
		}
	}

	s.log.Info("logged in")
	return nil
}

// ListOrders opens the purchase order history and returns its orders.
// Rows that cannot be parsed are logged and skipped.
func (s *Session) ListOrders(ctx context.Context) ([]order.PurchaseOrder, error) {
	if err := s.openOrdersPage(ctx); err != nil {
		return nil, err
	}

	// The orders view is a SPA: switching to the history tab does not navigate.
	err := browser.Retry(ctx, clickAttempts, retryDelay, func() error {
		return s.page.Click(ctx, historyTab)
	})
	if err != nil {
		return nil, fmt.Errorf("open history tab: %w", err)
	}

	rows, err := s.page.RowTexts(ctx, orderRows)
	if err != nil {
		return nil, fmt.Errorf("read orders table: %w", err)
	}

	orders := make([]order.PurchaseOrder, 0, len(rows))
	for i, cells := range rows {
		o, err := order.FromCells(cells)
		if err != nil {
			s.log.Warn("skipping unreadable order row", "row", i+1, "error", err)
			continue
		}
		orders = append(orders, o)
	}

	s.log.Debug("orders listed", "rows", len(rows), "orders", len(orders))
	return orders, nil
}

// DownloadReports opens the order and downloads its Excel reports to the
// browser's Downloads folder. After downloadSettle it moves them to
// <downloadDir>/<orderNumber>/ and returns their paths. It must be called
// while the list loaded by ListOrders is on screen. Afterwards the session
// goes back to the purchase orders page, even if a download failed.
func (s *Session) DownloadReports(ctx context.Context, orderNumber string) (files order.Files, err error) {
	log := s.log.With("order", orderNumber)

	err = s.page.ClickInRowAndWait(ctx, orderRows, orderNumberColumn, orderNumber, rowOpenButton)
	if err != nil {
		return order.Files{}, fmt.Errorf("open order: %w", err)
	}

	defer func() {
		if navErr := s.page.Navigate(ctx, ordersURL); navErr != nil {
			err = errors.Join(err, fmt.Errorf("return to purchase orders: %w", navErr))
		}
	}()

	for i := range reportsPerOrder {
		if err := s.downloadReport(ctx, i); err != nil {
			return order.Files{}, fmt.Errorf("report %d: %w", i+1, err)
		}
		log.Info("report download started", "report", i+1)
	}

	// The browser handles the downloads; give them time to finish before leaving the page.
	if err := browser.Sleep(ctx, downloadSettle); err != nil {
		return order.Files{}, err
	}

	downloads, err := browserDownloadsDir()
	if err != nil {
		return order.Files{}, err
	}
	return collectReports(downloads, filepath.Join(s.downloadDir, orderNumber), orderNumber)
}

// browserDownloadsDir is the folder where Chrome saves downloads by default.
func browserDownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find downloads folder: %w", err)
	}
	return filepath.Join(home, "Downloads"), nil
}

// collectReports finds the two reports of an order among the files the
// browser saved in src, by name (oc-<number>-products-details*.xlsx and
// oc-<number>-distributions-details*.xlsx), and moves them to dst. If a
// report was downloaded more than once ("... (1).xlsx"), the newest is used.
// A download still in progress (.crdownload) is not matched, so the order
// fails and is retried in the next cycle.
func collectReports(src, dst, orderNumber string) (order.Files, error) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return order.Files{}, fmt.Errorf("create order folder: %w", err)
	}

	var files order.Files
	for _, r := range []struct {
		kind string
		path *string
	}{
		{productsReport, &files.Products},
		{distributionReport, &files.Distribution},
	} {
		found, err := newest(filepath.Join(src, fmt.Sprintf("oc-%s-%s*.xlsx", orderNumber, r.kind)))
		if err != nil {
			return order.Files{}, err
		}
		to := filepath.Join(dst, filepath.Base(found))
		if err := moveFile(found, to); err != nil {
			return order.Files{}, err
		}
		*r.path = to
	}
	return files, nil
}

// newest returns the most recently modified file matching pattern.
func newest(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("search %s: %w", pattern, err)
	}
	var best string
	var bestTime time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestTime) {
			best, bestTime = m, info.ModTime()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no downloaded file matches %s", pattern)
	}
	return best, nil
}

// moveFile renames from to to, copying when they are on different drives.
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("move %s: %w", from, err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		return fmt.Errorf("move %s: %w", from, err)
	}
	return os.Remove(from)
}

// downloadReport clicks the index-th download button and picks the Excel option.
func (s *Session) downloadReport(ctx context.Context, index int) error {
	err := browser.Retry(ctx, clickAttempts, retryDelay, func() error {
		return s.page.ClickNth(ctx, downloadButton, index)
	})
	if err != nil {
		return fmt.Errorf("click download button: %w", err)
	}

	err = browser.Retry(ctx, clickAttempts, retryDelay, func() error {
		return s.page.ClickText(ctx, excelOption)
	})
	if err != nil {
		return fmt.Errorf("choose %s option: %w", excelOption, err)
	}
	return nil
}

func (s *Session) openOrdersPage(ctx context.Context) error {
	url, err := s.page.URL(ctx)
	if err != nil {
		return err
	}
	if url == ordersURL {
		return nil
	}
	if err := s.page.ClickAndWait(ctx, ordersCard); err != nil {
		return fmt.Errorf("open purchase orders card: %w", err)
	}
	return nil
}
