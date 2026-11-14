package falabella

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/Khaym03/falatracker/internal/falabella/falabellatest"
	"github.com/Khaym03/falatracker/internal/order"
	"github.com/Khaym03/falatracker/internal/standard"
)

const templatePath = "../../templates/standard.xlsx"

func newConverter(t *testing.T) *Converter {
	t.Helper()
	tpl, err := standard.LoadTemplate(templatePath)
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	return NewConverter(tpl, PlaceholderCustomer(), NoCatalog{})
}

// End to end: fake reports in, standard file out.
func TestConverterConvert(t *testing.T) {
	dir := t.TempDir()
	o, dist := falabellatest.Sample()
	files := order.Files{
		Products:     falabellatest.WriteProducts(t, dir, o),
		Distribution: falabellatest.WriteDistribution(t, dir, o.Number, dist),
	}

	rec, err := newConverter(t).Convert(files)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	wantStandard := filepath.Join(dir, "OC_FALABELLA_1000001_3-11-2026.xlsx")
	if rec.OrderNumber != "1000001" || rec.BillingDate != "2026-11-03" || rec.Files.Standard != wantStandard {
		t.Errorf("record = %+v", rec)
	}
	if rec.Files.Products != files.Products || rec.Files.Distribution != files.Distribution {
		t.Errorf("record lost the report paths: %+v", rec.Files)
	}
	if !slices.Equal(rec.Issues, []string{"Datos de cliente pendientes: RUC/NIT, condición de pago, IVA"}) {
		t.Errorf("issues = %q", rec.Issues)
	}

	// Check a few cells of the written file by header.
	cells := readByHeader(t, rec.Files.Standard)
	if len(cells) != 3 {
		t.Fatalf("standard file has %d rows, want 3", len(cells))
	}
	row := cells[1]
	for header, want := range map[string]string{
		standard.HeaderOrderNumber:  "1000001",
		standard.HeaderProductCode:  "200002",
		standard.HeaderBrand:        "MARCA A",
		standard.HeaderQuantity:     "3",
		standard.HeaderOrderDate:    "03/11/2026",
		standard.HeaderDeliveryDate: "11/11/2026",
		standard.HeaderCurrency:     "COP",
		standard.HeaderTaxID:        Pending,
	} {
		if row[header] != want {
			t.Errorf("%s = %q, want %q", header, row[header], want)
		}
	}
}

// The billing date is process data: it goes to the record (read by PAD via
// GET /orders), never to the standard file loaded into Dynamics.
func TestConverterBillingDateGoesToTheRecordOnly(t *testing.T) {
	dir := t.TempDir()
	o, _ := falabellatest.Sample()
	o.IssueDate, o.DeliveryEnd = "29-10-2026", "06-11-2026" // delivered next month

	rec, err := newConverter(t).Convert(order.Files{Products: falabellatest.WriteProducts(t, dir, o)})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if rec.BillingDate != "2026-11-01" {
		t.Errorf("BillingDate = %q, want 2026-11-01", rec.BillingDate)
	}
	for _, row := range readByHeader(t, rec.Files.Standard) {
		for _, h := range []string{"FECHA FACTURACIÓN", "FECHA INICIO ENTREGA"} {
			if _, ok := row[h]; ok {
				t.Fatalf("standard file must not have %q", h)
			}
		}
	}
}

func TestConverterConvertToExplicitPath(t *testing.T) {
	dir := t.TempDir()
	o, _ := falabellatest.Sample()
	out := filepath.Join(dir, "sub", "out.xlsx")

	rec, err := newConverter(t).ConvertTo(order.Files{Products: falabellatest.WriteProducts(t, dir, o)}, out)
	if err != nil {
		t.Fatalf("ConvertTo: %v", err)
	}
	if rec.Files.Standard != out {
		t.Errorf("Standard = %q, want %q", rec.Files.Standard, out)
	}
}

func TestConverterFailsOnUnreadableReport(t *testing.T) {
	_, err := newConverter(t).Convert(order.Files{Products: filepath.Join(t.TempDir(), "missing.xlsx")})
	if err == nil {
		t.Fatal("expected an error")
	}
}

// readByHeader returns the data rows of a standard file as header -> raw value.
func readByHeader(t *testing.T, path string) []map[string]string {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0), excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	headers := rows[1]
	var out []map[string]string
	for _, r := range rows[2:] {
		m := make(map[string]string)
		for i, v := range r {
			if i < len(headers) {
				m[headers[i]] = v
			}
		}
		out = append(out, m)
	}
	return out
}
