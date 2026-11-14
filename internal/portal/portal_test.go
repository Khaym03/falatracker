package portal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Khaym03/falatracker/internal/browser"
	"github.com/Khaym03/falatracker/internal/browsertest"
	"github.com/Khaym03/falatracker/internal/order"
)

func row(cells ...string) string {
	var b strings.Builder
	b.WriteString("<tr>")
	for _, c := range cells {
		fmt.Fprintf(&b, "<td>%s</td>", c)
	}
	b.WriteString("<td><button>open</button></td></tr>")
	return b.String()
}

// fakeOrdersPage mimics the portal's purchase orders SPA: the history rows
// are only rendered after clicking the second tab.
func fakeOrdersPage() string {
	history := row("FALABELLA", "4500001", "01/11/2026", "02/11/2026", "03/11/2026", "Directo", "CD", "$100", "5", "0%", "Pending") +
		row("SODIMAC", "4500002", "01/11/2026", "02/11/2026", "03/11/2026", "Directo", "CD", "$200", "7", "12,5%", "Canceled") +
		"<tr><td colspan=11>malformed row</td></tr>" +
		row("TOTTUS", "4500003", "01/11/2026", "02/11/2026", "03/11/2026", "Directo", "CD", "$300", "9", "n/a", "Pending")

	return `<!doctype html><html><body>
<div id="single-spa-application:purchaseOrders">
  <div aria-label="po tabs" role="tablist">
    <button>Pending</button>
    <button onclick="document.querySelector('tbody').innerHTML = HISTORY">History</button>
  </div>
  <table aria-labelledby="tableTitle"><tbody></tbody></table>
</div>
<script>const HISTORY = ` + "`" + history + "`" + `</script>
</body></html>`
}

func TestListOrders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<a href="/purchase-orders">Purchase orders</a>`)
	})
	mux.HandleFunc("/purchase-orders", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, fakeOrdersPage())
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := browsertest.New(t)
	rp := browsertest.Page(t, b, srv.URL)
	s := newSession(browser.NewPage(rp, 10*time.Second), Credential{}, t.TempDir(), slog.New(slog.DiscardHandler))

	orders, err := s.ListOrders(context.Background())
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}

	// The malformed row and the one with an invalid percentage are skipped.
	want := []order.PurchaseOrder{
		{
			BU: "FALABELLA", OrderNumber: "4500001", CreationDate: "01/11/2026",
			DeliveryStartDate: "02/11/2026", DeliveryEndDate: "03/11/2026",
			DeliveryType: "Directo", Delivery: "CD", TotalAmount: "$100", TotalQuantity: "5",
			AgreementPercentage: 0, Status: "Pending",
		},
		{
			BU: "SODIMAC", OrderNumber: "4500002", CreationDate: "01/11/2026",
			DeliveryStartDate: "02/11/2026", DeliveryEndDate: "03/11/2026",
			DeliveryType: "Directo", Delivery: "CD", TotalAmount: "$200", TotalQuantity: "7",
			AgreementPercentage: 12.5, Status: "Canceled",
		},
	}
	if len(orders) != len(want) {
		t.Fatalf("got %d orders, want %d: %+v", len(orders), len(want), orders)
	}
	for i := range want {
		if orders[i] != want[i] {
			t.Errorf("order %d = %+v, want %+v", i, orders[i], want[i])
		}
	}
}

// The reports are found in the browser's Downloads folder by name and moved
// to the order folder.
func TestCollectReports(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "1000001")
	write := func(name string, age time.Duration) {
		t.Helper()
		p := filepath.Join(src, name)
		if err := os.WriteFile(p, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := time.Now().Add(-age)
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	write("oc-1000001-products-details-07-11-2026.xlsx", time.Hour) // older download
	write("oc-1000001-products-details-07-11-2026 (1).xlsx", 0)     // newest: chosen
	write("oc-1000001-distributions-details-07-11-2026.xlsx", 0)
	write("oc-2000002-products-details-07-11-2026.xlsx", 0)                 // another order
	write("oc-1000001-distributions-details-08-11-2026.xlsx.crdownload", 0) // in progress

	files, err := collectReports(src, dst, "1000001")
	if err != nil {
		t.Fatalf("collectReports: %v", err)
	}
	if want := filepath.Join(dst, "oc-1000001-products-details-07-11-2026 (1).xlsx"); files.Products != want {
		t.Errorf("Products = %q, want %q", files.Products, want)
	}
	if want := filepath.Join(dst, "oc-1000001-distributions-details-07-11-2026.xlsx"); files.Distribution != want {
		t.Errorf("Distribution = %q, want %q", files.Distribution, want)
	}
	for _, p := range []string{files.Products, files.Distribution} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("not moved: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(src, "oc-2000002-products-details-07-11-2026.xlsx")); err != nil {
		t.Error("files of other orders must not be touched")
	}
}

func TestCollectReportsMissing(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "oc-1000001-products-details-07-11-2026.xlsx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := collectReports(src, t.TempDir(), "1000001")
	if err == nil || !strings.Contains(err.Error(), "distributions-details") {
		t.Fatalf("error = %v, want missing distribution report", err)
	}
}

func TestCredentialValidate(t *testing.T) {
	tests := []struct {
		cred    Credential
		wantErr string
	}{
		{Credential{User: "u", Password: "p"}, ""},
		{Credential{User: "u"}, "missing password"},
		{Credential{Password: "p"}, "missing user"},
		{Credential{}, "missing user and password"},
	}
	for _, tt := range tests {
		err := tt.cred.Validate()
		if tt.wantErr == "" {
			if err != nil {
				t.Errorf("Validate(%+v) = %v, want nil", tt.cred, err)
			}
			continue
		}
		if err == nil || err.Error() != tt.wantErr {
			t.Errorf("Validate(%+v) = %v, want %q", tt.cred, err, tt.wantErr)
		}
	}
}
