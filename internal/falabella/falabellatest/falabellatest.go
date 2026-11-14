// Package falabellatest builds fake Falabella reports for tests.
//
// The column layout is copied from real reports downloaded in November 2026;
// every value is invented, because the repository is public and real
// purchase orders contain commercial data (costs, stores, tax ids).
package falabellatest

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// ProductsHeaders is the header row of a products-details report.
var ProductsHeaders = []string{
	"Ítem", "Número OC", "BU", "Fecha de descarga", "Estado OC", "Tipo de orden",
	"Tax id proveedor", "Razón social", "Fecha de emisión", "Forma de pago", "Moneda",
	"Total OC bruto", "Total cargos", "Total descuentos", "IVA", "Total OC neto",
	"Fecha inicio recepción", "Fecha fin recepción", "UPC", "SKU", "Descripción", "VIN",
	"Marca", "Modelo", "ID subclasificación", "Descripción subclasificación",
	"ID subclasificación", "Descripción subclasificación", "ID subclasificación",
	"Descripción subclasificación", "ID subclasificación", "Descripción subclasificación",
	"ID clasificación", "Descripción clasificación", "Precio de venta", "Moneda precio de venta",
	"Costo", "Moneda costo", "Tamaño", "Color", "Unidades compradas", "Unidades canceladas",
	"Unidades recibidas", "Cargos línea", "Descuentos línea", "Unidad de medida",
	"Dimensión logística", "Unidades dimensión logística",
}

// DistributionHeaders is the header row of a distributions-details report.
var DistributionHeaders = []string{
	"Ítem", "Fecha de descarga", "Número OC", "Estado OC", "Tipo de orden", "Fecha de emisión",
	"Fecha inicio recepción", "Fecha fin recepción", "Tax id proveedor", "Razón social",
	"ID lugar de entrega", "Descripción lugar de entrega", "Dirección de entrega", "UPC", "SKU",
	"Modelo", "VIN", "Unidad de medida", "Nro capas palet TI", "Empaq capa palet HI",
	"Id de local", "Local", "Unidades compradas", "Unidades recibidas", "Cantidad de empaque",
	"Pasillo", "Lineal", "Metro",
}

// Order is the data of a fake products-details report. Values are strings,
// as in the real files.
type Order struct {
	Number        string
	IssueDate     string // dd-mm-yyyy
	DeliveryStart string
	DeliveryEnd   string
	PaymentTerms  string
	Currency      string
	GrossTotal    string
	Products      []Product
}

// Product is a row of a fake products-details report.
type Product struct {
	UPC, SKU, Description, VIN, Brand, Classification string
	Cost, Purchased, Cancelled, Charges, Discounts    string
}

// Allocation is a row of a fake distributions-details report.
type Allocation struct {
	SKU, VIN, StoreID, Store, Units string
}

// Sample returns a consistent order of 3 products (one with a 13-digit VIN,
// two brands) and its distribution across stores. Gross total: 135000.
func Sample() (Order, []Allocation) {
	o := Order{
		Number:        "1000001",
		IssueDate:     "03-11-2026",
		DeliveryStart: "04-11-2026",
		DeliveryEnd:   "11-11-2026",
		Currency:      "COP",
		GrossTotal:    "135000",
		Products: []Product{
			{UPC: "7700000000017", SKU: "90000001", Description: "CREMA DEMO 50 ML", VIN: "100001",
				Brand: "MARCA A", Classification: "HIDRATACION", Cost: "10000", Purchased: "2"},
			{UPC: "7700000000024", SKU: "90000002", Description: "GEL DEMO 200 ML", VIN: "7700000200002",
				Brand: "MARCA A", Classification: "LIMPIEZA", Cost: "25000", Purchased: "3"},
			{UPC: "7700000000031", SKU: "90000003", Description: "PROTECTOR DEMO 40 ML", VIN: "300003",
				Brand: "MARCA B", Classification: "SOLARES", Cost: "40000", Purchased: "1"},
		},
	}
	dist := []Allocation{
		{SKU: "90000001", VIN: "100001", StoreID: "35", Store: "TIENDA NORTE", Units: "1"},
		{SKU: "90000001", VIN: "100001", StoreID: "82", Store: "TIENDA SUR", Units: "1"},
		{SKU: "90000002", VIN: "7700000200002", StoreID: "35", Store: "TIENDA NORTE", Units: "3"},
		{SKU: "90000003", VIN: "300003", StoreID: "82", Store: "TIENDA SUR", Units: "1"},
	}
	return o, dist
}

// WriteProducts writes o as dir/oc-<number>-products-details-07-11-2026.xlsx
// and returns the path.
func WriteProducts(t *testing.T, dir string, o Order) string {
	t.Helper()
	rows := make([][]string, 0, len(o.Products))
	for i, p := range o.Products {
		v := map[string]string{
			"Ítem": fmt.Sprint(i + 1), "Número OC": o.Number, "BU": "FAL-CO",
			"Fecha de descarga": "07-11-2026", "Estado OC": "Cursado", "Tipo de orden": "Cross-Dock",
			"Tax id proveedor": "900000000-0", "Razón social": "PROVEEDOR DEMO SAS",
			"Fecha de emisión": o.IssueDate, "Forma de pago": o.PaymentTerms, "Moneda": o.Currency,
			"Total OC bruto": o.GrossTotal, "Total cargos": "0", "Total descuentos": "0",
			"Fecha inicio recepción": o.DeliveryStart, "Fecha fin recepción": o.DeliveryEnd,
			"UPC": p.UPC, "SKU": p.SKU, "Descripción": p.Description, "VIN": p.VIN, "Marca": p.Brand,
			"Modelo": p.VIN, "Descripción clasificación": p.Classification,
			"Precio de venta": "99999", "Moneda precio de venta": o.Currency,
			"Costo": p.Cost, "Moneda costo": o.Currency, "Unidades compradas": p.Purchased,
			"Unidades canceladas": def(p.Cancelled, "0"), "Unidades recibidas": "0",
			"Cargos línea": def(p.Charges, "0"), "Descuentos línea": def(p.Discounts, "0"),
			"Unidad de medida": "UNI", "Unidades dimensión logística": "1",
		}
		rows = append(rows, byHeader(ProductsHeaders, v))
	}
	path := filepath.Join(dir, fmt.Sprintf("oc-%s-products-details-07-11-2026.xlsx", o.Number))
	WriteXLSX(t, path, ProductsHeaders, rows)
	return path
}

// WriteDistribution writes the allocations of an order as
// dir/oc-<number>-distributions-details-07-11-2026.xlsx and returns the path.
func WriteDistribution(t *testing.T, dir, number string, dist []Allocation) string {
	t.Helper()
	rows := make([][]string, 0, len(dist))
	for i, a := range dist {
		v := map[string]string{
			"Ítem": fmt.Sprint(i + 1), "Número OC": number, "Estado OC": "Cursado",
			"ID lugar de entrega": "8030", "Descripción lugar de entrega": "CD DEMO",
			"SKU": a.SKU, "VIN": a.VIN, "Modelo": "DEMO", "Id de local": a.StoreID, "Local": a.Store,
			"Unidades compradas": a.Units, "Unidades recibidas": "0", "Cantidad de empaque": "1",
		}
		rows = append(rows, byHeader(DistributionHeaders, v))
	}
	path := filepath.Join(dir, fmt.Sprintf("oc-%s-distributions-details-07-11-2026.xlsx", number))
	WriteXLSX(t, path, DistributionHeaders, rows)
	return path
}

// WriteXLSX writes a single-sheet workbook with a header row and text cells,
// like the reports Falabella produces.
func WriteXLSX(t *testing.T, path string, headers []string, rows [][]string) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)
	for r, row := range append([][]string{headers}, rows...) {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellStr(sheet, cell, v); err != nil {
				t.Fatalf("write %s: %v", cell, err)
			}
		}
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save %s: %v", path, err)
	}
}

// byHeader builds a row in header order; the first occurrence of a
// duplicated header gets the value.
func byHeader(headers []string, values map[string]string) []string {
	row := make([]string, len(headers))
	seen := make(map[string]bool)
	for i, h := range headers {
		if !seen[h] {
			row[i] = values[h]
			seen[h] = true
		}
	}
	return row
}

func def(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
