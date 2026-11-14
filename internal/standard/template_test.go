package standard

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

const repoTemplate = "../../templates/standard.xlsx"

func TestRepoTemplateHasEveryKnownColumn(t *testing.T) {
	tpl, err := LoadTemplate(repoTemplate)
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	for _, c := range columns {
		if !tpl.Has(c.header) {
			t.Errorf("repo template lacks %q", c.header)
		}
	}
}

func TestLoadTemplateMissingRequiredColumns(t *testing.T) {
	path := writeTemplate(t, []string{"RUC", "N° ORDEN", "CANTIDAD"})

	_, err := LoadTemplate(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, h := range []string{HeaderProductCode, HeaderUnitPrice} {
		if !strings.Contains(err.Error(), h) {
			t.Errorf("error %q should name %q", err, h)
		}
	}
}

func TestLoadTemplateWithoutHeaderRow(t *testing.T) {
	f := excelize.NewFile()
	path := filepath.Join(t.TempDir(), "empty.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTemplate(path); err == nil {
		t.Fatal("expected an error")
	}
}

func sampleLine() Line {
	vat := 19.0
	return Line{
		TaxID: "800000000-1", CustomerName: "CLIENTE DEMO", PaymentTerms: "60 DIAS",
		OrderNumber: "1000001", OrderDate: date3("03-11-2026"),
		DeliveryDate: date3("11-11-2026"), Currency: "COP",
		Brand: "MARCA A", ProductCode: "012345", Barcode: "7700000000017", Description: "CREMA DEMO",
		Quantity: 2, UnitPrice: 10000, Subtotal: 20000, VATPct: &vat, Total: 23800,
		SourceDocument: "oc.xlsx", Observation: "obs", Error: "err",
	}
}

func date3(s string) time.Time {
	t, _ := time.Parse("02-01-2006", s)
	return t
}

func TestWriteRepoTemplate(t *testing.T) {
	tpl, err := LoadTemplate(repoTemplate)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "nested", "out.xlsx")

	second := sampleLine()
	second.ProductCode, second.VATPct = "067890", nil
	second.Catalog = &CatalogMatch{ItemNo: "A1", ParentCode: "P1", Price: 12500, DiscountPct: 20, Subtotal: 20000}
	if err := tpl.Write(out, []Line{sampleLine(), second}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	rows := readRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("got %d data rows, want 2", len(rows))
	}
	first := rows[0]
	for h, want := range map[string]string{
		HeaderTaxID:        "800000000-1",
		HeaderOrderNumber:  "1000001",
		HeaderOrderDate:    "03/11/2026", // dates are text dd/mm/yyyy
		HeaderDeliveryDate: "11/11/2026",
		HeaderProductCode:  "012345", // leading zero kept (text)
		HeaderQuantity:     "2",
		HeaderUnitPrice:    "10000",
		HeaderVAT:          "19",
		HeaderTotal:        "23800",
		HeaderCurrency:     "COP",
		HeaderItemNo:       "", // no catalog match -> empty
		HeaderNaosPrice:    "",
		HeaderSize:         "", // not provided -> empty
		HeaderError:        "err",
	} {
		if first[h] != want {
			t.Errorf("row 1 %s = %q, want %q", h, first[h], want)
		}
	}
	if rows[1][HeaderVAT] != "" {
		t.Errorf("unknown VAT should be empty, got %q", rows[1][HeaderVAT])
	}
	if rows[1][HeaderItemNo] != "A1" || rows[1][HeaderNaosPrice] != "12500" {
		t.Errorf("catalog columns = %q / %q", rows[1][HeaderItemNo], rows[1][HeaderNaosPrice])
	}

	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	// Totals row untouched.
	if formula, _ := f.GetCellFormula(sheet, "Q1"); formula != "SUM(Q3:Q1048576)" {
		t.Errorf("Q1 formula = %q", formula)
	}
	// Every data row gets the prototype (row 3) style.
	for _, col := range []string{"N", "Q"} {
		s3, _ := f.GetCellStyle(sheet, col+"3")
		s4, _ := f.GetCellStyle(sheet, col+"4")
		if s3 == 0 || s3 != s4 {
			t.Errorf("column %s: style row 3 = %d, row 4 = %d", col, s3, s4)
		}
	}
}

// The schema is not frozen: the template can reorder columns and add
// unknown ones; values must follow the headers.
func TestWriteFollowsHeadersNotPositions(t *testing.T) {
	headers := []string{"  cantidad ", "COLUMNA NUEVA", "PRECIO UNIT.", "CÓD. PRODUCTO", "N° ORDEN"}
	tpl, err := LoadTemplate(writeTemplate(t, headers))
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if tpl.Has(HeaderTaxID) {
		t.Error("RUC is not in this template")
	}

	out := filepath.Join(t.TempDir(), "out.xlsx")
	if err := tpl.Write(out, []Line{sampleLine()}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, _ := f.GetRows(f.GetSheetName(0), excelize.Options{RawCellValue: true})
	want := []string{"2", "", "10000", "012345", "1000001"}
	if strings.Join(got[2], "|") != strings.Join(want, "|") {
		t.Errorf("row 3 = %q, want %q", got[2], want)
	}
}

func TestWriteWithoutLines(t *testing.T) {
	tpl, err := LoadTemplate(repoTemplate)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "empty.xlsx")
	if err := tpl.Write(out, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if rows := readRows(t, out); len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

// writeTemplate creates a minimal template: empty totals row, headers in row 2.
func writeTemplate(t *testing.T, headers []string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		if err := f.SetCellStr(sheet, cell, h); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "template.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

// readRows returns the data rows of a written file as header -> raw value.
func readRows(t *testing.T, path string) []map[string]string {
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
	headers := rows[headerRow-1]
	var out []map[string]string
	for _, r := range rows[firstDataRow-1:] {
		if strings.Join(r, "") == "" {
			continue
		}
		m := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(r) {
				m[h] = r[i]
			} else {
				m[h] = ""
			}
		}
		out = append(out, m)
	}
	return out
}
