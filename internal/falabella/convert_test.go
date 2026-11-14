package falabella

import (
	"slices"
	"strings"
	"testing"

	"github.com/Khaym03/falatracker/internal/standard"
)

// sampleOrder is the order of falabellatest.Sample, already parsed.
func sampleOrder() (Order, []Allocation) {
	o := Order{
		Number: "1000001", BU: "FAL-CO", Currency: "COP", GrossTotal: 135000,
		IssueDate: d("03-11-2026"), DeliveryEnd: d("11-11-2026"),
		Products: []Product{
			{UPC: "7700000000017", SKU: "90000001", Description: "CREMA DEMO 50 ML", VIN: "100001", Brand: "MARCA A", Classification: "HIDRATACION", Cost: 10000, Purchased: 2},
			{UPC: "7700000000024", SKU: "90000002", Description: "GEL DEMO 200 ML", VIN: "7700000200002", Brand: "MARCA A", Classification: "LIMPIEZA", Cost: 25000, Purchased: 3},
			{UPC: "7700000000031", SKU: "90000003", Description: "PROTECTOR DEMO 40 ML", VIN: "300003", Brand: "MARCA B", Classification: "SOLARES", Cost: 40000, Purchased: 1},
		},
	}
	dist := []Allocation{
		{SKU: "90000001", Units: 1}, {SKU: "90000001", Units: 1},
		{SKU: "90000002", Units: 3}, {SKU: "90000003", Units: 1},
	}
	return o, dist
}

func realCustomer() Customer {
	vat := 19.0
	return Customer{TaxID: "800000000-1", Name: "FALABELLA DEMO", PaymentTerms: "60 DIAS", VATPct: &vat}
}

// fakeCatalog knows the products by code.
type fakeCatalog map[string]Item

func (c fakeCatalog) Lookup(code string) (Item, bool) {
	it, ok := c[code]
	return it, ok
}

func TestConvertMapsEveryColumn(t *testing.T) {
	o, dist := sampleOrder()
	res := Convert(o, dist, "oc-1000001-products-details.xlsx", realCustomer(), NoCatalog{})

	if len(res.Lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(res.Lines))
	}

	l := res.Lines[1]
	vat := 19.0
	want := standard.Line{
		TaxID: "800000000-1", CustomerName: "FALABELLA DEMO", PaymentTerms: "60 DIAS",
		OrderNumber: "1000001", OrderDate: d("03-11-2026"),
		DeliveryDate: d("11-11-2026"), Currency: "COP",
		Brand: "MARCA A", ProductCode: "200002", Barcode: "7700000000024",
		Description: "GEL DEMO 200 ML", Category: "LIMPIEZA",
		Quantity: 3, UnitPrice: 25000, Subtotal: 75000, VATPct: &vat, Total: 89250,
		SourceDocument: "oc-1000001-products-details.xlsx",
		Observation:    "No encontrado en maestro (Búsqueda: 200002)",
		Error:          "Sin código 'No' en la fila",
	}
	if l.VATPct == nil || *l.VATPct != 19 {
		t.Errorf("VATPct = %v, want 19", l.VATPct)
	}
	l.VATPct, want.VATPct = nil, nil // compared above; pointers differ
	if l != want {
		t.Errorf("line 2 =\n%+v\nwant\n%+v", l, want)
	}

	// With complete customer data and matching totals there are no issues.
	if len(res.Issues) != 0 {
		t.Errorf("issues = %v, want none", res.Issues)
	}
}

func TestConvertWithPlaceholderCustomer(t *testing.T) {
	o, dist := sampleOrder()
	res := Convert(o, dist, "x.xlsx", PlaceholderCustomer(), NoCatalog{})

	l := res.Lines[0]
	if l.TaxID != Pending || l.PaymentTerms != Pending || l.VATPct != nil {
		t.Errorf("placeholder values not used: %+v", l)
	}
	// Unknown VAT: IVA empty and TOTAL = SUBTOTAL (as in the existing files).
	if l.Total != l.Subtotal {
		t.Errorf("Total = %v, want Subtotal %v", l.Total, l.Subtotal)
	}
	want := []string{"Datos de cliente pendientes: RUC/NIT, condición de pago, IVA"}
	if !slices.Equal(res.Issues, want) {
		t.Errorf("issues = %q, want %q", res.Issues, want)
	}
}

func TestConvertPaymentTermsFromReportWin(t *testing.T) {
	o, dist := sampleOrder()
	o.PaymentTerms = "CREDITO 90"
	res := Convert(o, dist, "x.xlsx", realCustomer(), NoCatalog{})
	if got := res.Lines[0].PaymentTerms; got != "CREDITO 90" {
		t.Errorf("PaymentTerms = %q, want the report value", got)
	}
}

func TestConvertWithCatalog(t *testing.T) {
	o, dist := sampleOrder()
	catalog := fakeCatalog{
		// 12500 - 20% = 10000: matches the document exactly.
		"100001": {ItemNo: "A0001", ParentCode: "P0001", Price: 12500, DiscountPct: 20},
		// 31300 - 20% = 25040 vs 25000: difference beyond tolerance.
		"200002": {ItemNo: "A0002", ParentCode: "P0002", Price: 31300, DiscountPct: 20},
	}
	res := Convert(o, dist, "x.xlsx", realCustomer(), catalog)

	ok := res.Lines[0]
	if ok.Catalog == nil || ok.Catalog.ItemNo != "A0001" || ok.Catalog.Subtotal != 20000 || ok.Catalog.Difference != 0 {
		t.Errorf("line 1 catalog = %+v", ok.Catalog)
	}
	if ok.Error != "" || ok.Observation != "" {
		t.Errorf("line 1 should be clean, got error=%q obs=%q", ok.Error, ok.Observation)
	}

	diff := res.Lines[1]
	if diff.Catalog == nil || diff.Catalog.Difference != -120 {
		t.Errorf("line 2 catalog = %+v, want difference -120", diff.Catalog)
	}
	if diff.Error != "Diferencia de subtotal: doc=75000.00 vs naos=75120.00" {
		t.Errorf("line 2 error = %q", diff.Error)
	}

	missing := res.Lines[2]
	if missing.Catalog != nil || !strings.Contains(missing.Observation, "No encontrado en maestro (Búsqueda: 300003)") {
		t.Errorf("line 3 = %+v", missing)
	}
}

func TestConvertLineProblems(t *testing.T) {
	o, dist := sampleOrder()
	o.Products[0].Cancelled = 1
	o.Products[1].LineDiscounts = 500
	o.Products[2].VIN = "123"
	cat := fakeCatalog{
		"100001": {Price: 10000}, "200002": {Price: 25000},
	}
	res := Convert(o, dist, "x.xlsx", realCustomer(), cat)

	if got := res.Lines[0].Observation; got != "Unidades canceladas: 1" {
		t.Errorf("cancelled units observation = %q", got)
	}
	if got := res.Lines[1].Error; !strings.Contains(got, "descuentos=500.00") {
		t.Errorf("line discount error = %q", got)
	}
	if l := res.Lines[2]; l.ProductCode != "" || !strings.Contains(l.Error, "menos de 6 dígitos") {
		t.Errorf("invalid VIN line = %+v", l)
	}
}

func TestConvertOrderChecks(t *testing.T) {
	t.Run("gross total mismatch", func(t *testing.T) {
		o, dist := sampleOrder()
		o.GrossTotal = 140000
		res := Convert(o, dist, "x.xlsx", realCustomer(), NoCatalog{})
		want := "Suma de subtotales (135000.00) distinta al Total OC bruto (140000.00)"
		if !slices.Contains(res.Issues, want) {
			t.Errorf("issues = %q, want %q", res.Issues, want)
		}
	})

	t.Run("distribution does not add up", func(t *testing.T) {
		o, dist := sampleOrder()
		dist = dist[:2]                                            // SKU 90000002 and 90000003 missing
		dist = append(dist, Allocation{SKU: "99999999", Units: 4}) // unknown SKU
		res := Convert(o, dist, "x.xlsx", realCustomer(), NoCatalog{})
		want := []string{
			"SKU 90000002: 3 unidades compradas vs 0 en distribución",
			"SKU 90000003: 1 unidades compradas vs 0 en distribución",
			"SKU 99999999: 0 unidades compradas vs 4 en distribución",
		}
		if !slices.Equal(res.Issues, want) {
			t.Errorf("issues =\n%q\nwant\n%q", res.Issues, want)
		}
	})

	t.Run("no distribution file", func(t *testing.T) {
		o, _ := sampleOrder()
		res := Convert(o, nil, "x.xlsx", realCustomer(), NoCatalog{})
		if len(res.Issues) != 1 || !strings.HasPrefix(res.Issues[0], "Sin archivo de distribución") {
			t.Errorf("issues = %q", res.Issues)
		}
	})
}
