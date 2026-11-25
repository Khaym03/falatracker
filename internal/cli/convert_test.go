package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Khaym03/falatracker/internal/falabella/falabellatest"
	"github.com/Khaym03/falatracker/internal/order"
)

const testTemplate = "../../templates/standard.xlsx"

func runConvert(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd(nil)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"convert", "--template", testTemplate}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestConvertCommand(t *testing.T) {
	dir := t.TempDir()
	o, dist := falabellatest.Sample()
	products := falabellatest.WriteProducts(t, dir, o)
	distribution := falabellatest.WriteDistribution(t, dir, o.Number, dist)
	out := filepath.Join(dir, "out.xlsx")

	stdout, err := runConvert(t,
		"--products", products, "--distribution", distribution, "-o", out,
		"--customer-tax-id", "800000000-1", "--payment-terms", "60 DIAS", "--vat", "19",
		"--min-order-amount", "0",
	)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	var rec order.Record
	if err := json.Unmarshal([]byte(stdout), &rec); err != nil {
		t.Fatalf("output is not a JSON record: %v\n%s", err, stdout)
	}
	if rec.OrderNumber != "1000001" || rec.BillingDate != "2026-11-03" || rec.Files.Standard != out {
		t.Errorf("record = %+v", rec)
	}
	// All customer data given: no issues.
	if len(rec.Issues) != 0 {
		t.Errorf("issues = %q", rec.Issues)
	}
	// convert does not touch the store, so the output has no status.
	if strings.Contains(stdout, "status") || strings.Contains(stdout, "processed_at") {
		t.Errorf("unexpected store fields in output:\n%s", stdout)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("standard file not written: %v", err)
	}
}

func TestConvertCommandWithPlaceholders(t *testing.T) {
	dir := t.TempDir()
	o, _ := falabellatest.Sample()
	stdout, err := runConvert(t, "--products", falabellatest.WriteProducts(t, dir, o))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	var rec order.Record
	if err := json.Unmarshal([]byte(stdout), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Status != order.StatusRejected {
		t.Errorf("status = %q, want %q", rec.Status, order.StatusRejected)
	}
	want := []string{
		"Pedido por debajo del mínimo de compra (135000.00 < 500000.00): no radicar",
		"Sin archivo de distribución: no se validaron unidades por tienda",
		"Datos de cliente pendientes: RUC/NIT, condición de pago",
	}
	if !slices.Equal(rec.Issues, want) {
		t.Errorf("issues = %q, want %q", rec.Issues, want)
	}
	// Default output: next to the products report.
	if want := filepath.Join(dir, "OC_FALABELLA_1000001_3-11-2026.xlsx"); rec.Files.Standard != want {
		t.Errorf("Standard = %q, want %q", rec.Files.Standard, want)
	}
}

func TestConvertCommandErrors(t *testing.T) {
	if _, err := runConvert(t); err == nil || !strings.Contains(err.Error(), "products") {
		t.Errorf("missing --products: error = %v", err)
	}
	if _, err := runConvert(t, "--products", filepath.Join(t.TempDir(), "missing.xlsx")); err == nil {
		t.Error("missing file: expected an error")
	}
	if _, err := runConvert(t, "--products", filepath.Join(t.TempDir(), "missing.xlsx"), "--min-order-amount", "-1"); err == nil || !strings.Contains(err.Error(), "min-order-amount") {
		t.Errorf("negative --min-order-amount: error = %v", err)
	}
}

func TestResolveTemplatePathFallback(t *testing.T) {
	installDir := t.TempDir()
	tplDir := filepath.Join(installDir, "templates")
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bundled := filepath.Join(tplDir, "standard.xlsx")
	if err := os.WriteFile(bundled, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeExe := func() (string, error) {
		return filepath.Join(installDir, "falatracker.exe"), nil
	}

	// Relative path not present in CWD falls back to executable directory.
	cwd := t.TempDir()
	t.Chdir(cwd)
	if got := resolveTemplatePathFrom("templates/standard.xlsx", fakeExe); got != bundled {
		t.Errorf("fallback = %q, want %q", got, bundled)
	}

	// Missing template returns the original path unchanged.
	if got := resolveTemplatePathFrom("templates/missing.xlsx", fakeExe); got != "templates/missing.xlsx" {
		t.Errorf("missing = %q, want original path", got)
	}

	// Existing relative path in CWD takes precedence over executable directory.
	localTplDir := filepath.Join(cwd, "templates")
	if err := os.MkdirAll(localTplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localTplDir, "standard.xlsx"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTemplatePathFrom("templates/standard.xlsx", fakeExe); got != "templates/standard.xlsx" {
		t.Errorf("cwd precedence = %q, want original relative path", got)
	}
}
