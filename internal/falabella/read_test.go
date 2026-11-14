package falabella

import (
	"slices"
	"strings"
	"testing"

	"github.com/Khaym03/falatracker/internal/falabella/falabellatest"
)

func TestReadProducts(t *testing.T) {
	o, _ := falabellatest.Sample()
	path := falabellatest.WriteProducts(t, t.TempDir(), o)

	got, err := ReadProducts(path)
	if err != nil {
		t.Fatalf("ReadProducts: %v", err)
	}

	if got.Number != "1000001" || got.BU != "FAL-CO" || got.Status != "Cursado" || got.Currency != "COP" {
		t.Errorf("order header = %+v", got)
	}
	if !got.IssueDate.Equal(d("03-11-2026")) || !got.DeliveryEnd.Equal(d("11-11-2026")) {
		t.Errorf("dates = %v %v", got.IssueDate, got.DeliveryEnd)
	}
	if got.GrossTotal != 135000 {
		t.Errorf("GrossTotal = %v, want 135000", got.GrossTotal)
	}
	if len(got.Products) != 3 {
		t.Fatalf("got %d products, want 3", len(got.Products))
	}

	want := Product{
		UPC: "7700000000024", SKU: "90000002", Description: "GEL DEMO 200 ML", VIN: "7700000200002",
		Brand: "MARCA A", Classification: "LIMPIEZA", Cost: 25000, Purchased: 3,
	}
	if got.Products[1] != want {
		t.Errorf("product 2 = %+v, want %+v", got.Products[1], want)
	}
}

// Falabella may reorder its columns: headers, not positions, are used.
func TestReadProductsWithReorderedColumns(t *testing.T) {
	o, _ := falabellatest.Sample()
	original := falabellatest.WriteProducts(t, t.TempDir(), o)
	expected, err := ReadProducts(original)
	if err != nil {
		t.Fatal(err)
	}

	tbl, err := readTable(original)
	if err != nil {
		t.Fatal(err)
	}
	headers := slices.Clone(falabellatest.ProductsHeaders)
	slices.Reverse(headers)
	rows := make([][]string, len(tbl.rows))
	for i, r := range tbl.rows {
		rows[i] = slices.Clone(r)
		slices.Reverse(rows[i])
	}
	reordered := t.TempDir() + "/reordered.xlsx"
	falabellatest.WriteXLSX(t, reordered, headers, rows)

	got, err := ReadProducts(reordered)
	if err != nil {
		t.Fatalf("ReadProducts: %v", err)
	}
	if got.Number != expected.Number || !slices.Equal(got.Products, expected.Products) {
		t.Errorf("reordered file read differently:\n got %+v\nwant %+v", got, expected)
	}
}

func TestReadProductsErrors(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		write   func() string
		wantErr string
	}{
		{
			name: "missing required column",
			write: func() string {
				p := dir + "/missing.xlsx"
				falabellatest.WriteXLSX(t, p, []string{"Número OC", "VIN"}, [][]string{{"1", "100001"}})
				return p
			},
			wantErr: "missing column(s): Fecha de emisión, Fecha fin recepción, Costo, Unidades compradas",
		},
		{
			name: "no products",
			write: func() string {
				p := dir + "/empty.xlsx"
				falabellatest.WriteXLSX(t, p, falabellatest.ProductsHeaders, nil)
				return p
			},
			wantErr: "no products",
		},
		{
			name: "invalid cost",
			write: func() string {
				o, _ := falabellatest.Sample()
				o.Products[0].Cost = "10.000,50"
				return falabellatest.WriteProducts(t, t.TempDir(), o)
			},
			wantErr: `Costo: invalid number "10.000,50"`,
		},
		{
			name: "invalid date",
			write: func() string {
				o, _ := falabellatest.Sample()
				o.IssueDate = "2026-11-03"
				return falabellatest.WriteProducts(t, t.TempDir(), o)
			},
			wantErr: "want dd-mm-yyyy",
		},
		{
			name:    "not an excel file",
			write:   func() string { return dir + "/does-not-exist.xlsx" },
			wantErr: "open",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadProducts(tt.write())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestReadProductsRejectsMixedOrders(t *testing.T) {
	o, _ := falabellatest.Sample()
	path := falabellatest.WriteProducts(t, t.TempDir(), o)

	tbl, _ := readTable(path)
	rows := tbl.rows
	rows[1][1] = "9999999" // "Número OC" of the second product
	mixed := t.TempDir() + "/mixed.xlsx"
	falabellatest.WriteXLSX(t, mixed, falabellatest.ProductsHeaders, rows)

	if _, err := ReadProducts(mixed); err == nil || !strings.Contains(err.Error(), `order "9999999"`) {
		t.Fatalf("error = %v, want mixed order error", err)
	}
}

func TestReadDistribution(t *testing.T) {
	o, dist := falabellatest.Sample()
	path := falabellatest.WriteDistribution(t, t.TempDir(), o.Number, dist)

	got, err := ReadDistribution(path)
	if err != nil {
		t.Fatalf("ReadDistribution: %v", err)
	}
	want := []Allocation{
		{SKU: "90000001", VIN: "100001", StoreID: "35", Store: "TIENDA NORTE", Units: 1},
		{SKU: "90000001", VIN: "100001", StoreID: "82", Store: "TIENDA SUR", Units: 1},
		{SKU: "90000002", VIN: "7700000200002", StoreID: "35", Store: "TIENDA NORTE", Units: 3},
		{SKU: "90000003", VIN: "300003", StoreID: "82", Store: "TIENDA SUR", Units: 1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("ReadDistribution =\n%+v\nwant\n%+v", got, want)
	}
}
