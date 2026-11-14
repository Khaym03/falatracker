package browser_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"

	"github.com/Khaym03/falatracker/internal/browser"
	"github.com/Khaym03/falatracker/internal/browsertest"
)

const testPage = `<!doctype html>
<html><body>
<input id="name">
<button id="show" onclick="out.textContent = document.getElementById('name').value">Show</button>
<div id="out"></div>

<button class="b" onclick="out.textContent = 'b0'">First</button>
<button class="b" onclick="out.textContent = 'b1'">Second</button>

<button id="open-menu" onclick="menu.style.display = 'block'">Menu</button>
<div id="menu" style="display:none"><span onclick="out.textContent = 'excel'">Excel</span></div>

<table><tbody>
  <tr><td>x</td><td> 100 </td><td><a href="/detail?n=100">open</a></td></tr>
  <tr><td>y</td><td>200</td><td><a href="/detail?n=200">open</a></td></tr>
</tbody></table>

<div id="late"></div>
<script>
  setTimeout(() => { late.innerHTML = '<span id="appeared">hi</span>' }, 300)
</script>
</body></html>`

func newTestPage(t *testing.T, timeout time.Duration) (*browser.Page, *rod.Page, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, testPage) })
	mux.HandleFunc("/detail", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<h1>detail %s</h1>", r.URL.Query().Get("n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	b := browsertest.New(t)
	rp := browsertest.Page(t, b, srv.URL)
	return browser.NewPage(rp, timeout), rp, srv.URL
}

func text(t *testing.T, rp *rod.Page, selector string) string {
	t.Helper()
	return rp.MustElement(selector).MustText()
}

func TestPageInputAndClick(t *testing.T) {
	p, rp, _ := newTestPage(t, 5*time.Second)
	ctx := context.Background()

	if err := p.Input(ctx, "#name", "hello"); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if err := p.Click(ctx, "#show"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if got := text(t, rp, "#out"); got != "hello" {
		t.Errorf("out = %q, want hello", got)
	}
}

func TestPageClickMissingElementTimesOut(t *testing.T) {
	p, _, _ := newTestPage(t, 300*time.Millisecond)

	start := time.Now()
	err := p.Click(context.Background(), "#does-not-exist")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "#does-not-exist") {
		t.Errorf("error should name the selector: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("Click took %v, timeout not respected", time.Since(start))
	}
}

func TestPageClickRespectsCancellation(t *testing.T) {
	p, _, _ := newTestPage(t, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)

	start := time.Now()
	if err := p.Click(ctx, "#does-not-exist"); err == nil {
		t.Fatal("expected an error")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("Click took %v after cancellation", time.Since(start))
	}
}

func TestPageClickNth(t *testing.T) {
	p, rp, _ := newTestPage(t, time.Second)
	ctx := context.Background()

	if err := p.ClickNth(ctx, "button.b", 1); err != nil {
		t.Fatalf("ClickNth: %v", err)
	}
	if got := text(t, rp, "#out"); got != "b1" {
		t.Errorf("out = %q, want b1", got)
	}
	if err := p.ClickNth(ctx, "button.b", 5); err == nil {
		t.Error("expected an error for an index out of range")
	}
}

func TestPageClickText(t *testing.T) {
	p, rp, _ := newTestPage(t, time.Second)
	ctx := context.Background()

	if err := p.ClickText(ctx, "Excel"); err == nil {
		t.Error("expected an error while the option is hidden")
	}

	if err := p.Click(ctx, "#open-menu"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if err := p.ClickText(ctx, "Excel"); err != nil {
		t.Fatalf("ClickText: %v", err)
	}
	if got := text(t, rp, "#out"); got != "excel" {
		t.Errorf("out = %q, want excel", got)
	}
}

func TestPageExists(t *testing.T) {
	p, _, _ := newTestPage(t, time.Second)
	ctx := context.Background()

	ok, err := p.Exists(ctx, "#appeared", 3*time.Second)
	if err != nil || !ok {
		t.Errorf("Exists(#appeared) = %v, %v; want true, nil", ok, err)
	}

	ok, err = p.Exists(ctx, "#never", 200*time.Millisecond)
	if err != nil || ok {
		t.Errorf("Exists(#never) = %v, %v; want false, nil", ok, err)
	}

	ok, err = p.ExistsXPath(ctx, "//button[@id='show']", time.Second)
	if err != nil || !ok {
		t.Errorf("ExistsXPath = %v, %v; want true, nil", ok, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Exists(cancelled, "#never", time.Second); err == nil {
		t.Error("Exists with a cancelled context should return an error")
	}
}

func TestPageRowTexts(t *testing.T) {
	p, _, _ := newTestPage(t, time.Second)

	rows, err := p.RowTexts(context.Background(), "tbody tr")
	if err != nil {
		t.Fatalf("RowTexts: %v", err)
	}
	want := [][]string{{"x", "100", "open"}, {"y", "200", "open"}}
	if !slices.EqualFunc(rows, want, slices.Equal) {
		t.Errorf("RowTexts = %q, want %q", rows, want)
	}
}

func TestPageDownload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<a id="dl" href="/file">download</a> <a id="broken" href="#">nothing</a>`)
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="oc-1-products-details-07-11-2026.xlsx"`)
		fmt.Fprint(w, "excel bytes")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := browsertest.New(t)
	p := browser.NewPage(browsertest.Page(t, b, srv.URL), 5*time.Second)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "1000001")

	path, err := p.Download(ctx, dir, 10*time.Second, func() error { return p.Click(ctx, "#dl") })
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if want := filepath.Join(dir, "oc-1-products-details-07-11-2026.xlsx"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "excel bytes" {
		t.Errorf("file content = %q, %v", data, err)
	}

	// An action that starts no download fails at the timeout instead of hanging.
	start := time.Now()
	_, err = p.Download(ctx, dir, 500*time.Millisecond, func() error { return p.Click(ctx, "#broken") })
	if err == nil {
		t.Fatal("expected an error when nothing is downloaded")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("Download took %v, timeout not respected", time.Since(start))
	}

	// An action error is returned as is.
	if _, err := p.Download(ctx, dir, time.Second, func() error { return errors.New("boom") }); err == nil || err.Error() != "boom" {
		t.Errorf("error = %v, want boom", err)
	}
}

func TestPageClickInRowAndWait(t *testing.T) {
	p, _, srvURL := newTestPage(t, 5*time.Second)
	ctx := context.Background()

	if err := p.ClickInRowAndWait(ctx, "tbody tr", 1, "200", "a"); err != nil {
		t.Fatalf("ClickInRowAndWait: %v", err)
	}
	url, err := p.URL(ctx)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if want := srvURL + "/detail?n=200"; url != want {
		t.Errorf("URL = %q, want %q", url, want)
	}

	if err := p.Navigate(ctx, srvURL); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if err := p.ClickInRowAndWait(ctx, "tbody tr", 1, "999", "a"); err == nil {
		t.Error("expected an error for a missing row")
	}
}
