// Package browsertest starts a headless browser for integration tests.
// Tests using it are skipped with `go test -short` or when no Chrome/Chromium
// is installed.
package browsertest

import (
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// New launches a headless browser that is closed when the test ends.
func New(t *testing.T) *rod.Browser {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping browser test in -short mode")
	}
	bin, ok := launcher.LookPath()
	if !ok {
		t.Skip("no Chrome/Chromium installed")
	}

	l := launcher.New().Bin(bin).Headless(true).NoSandbox(true).Leakless(false)
	u, err := l.Launch()
	if err != nil {
		t.Fatalf("launch browser: %v", err)
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		l.Kill()
		t.Fatalf("connect browser: %v", err)
	}

	t.Cleanup(func() {
		_ = b.Close()
		l.Cleanup()
	})
	return b
}

// Page opens a new tab on b at url.
func Page(t *testing.T, b *rod.Browser, url string) *rod.Page {
	t.Helper()
	p, err := b.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		t.Fatalf("open page: %v", err)
	}
	if err := p.WaitLoad(); err != nil {
		t.Fatalf("wait load: %v", err)
	}
	return p
}
