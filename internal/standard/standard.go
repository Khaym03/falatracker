// Package standard writes the "Excel estándar": the common purchase order
// format, one row per product, that is loaded into Dynamics 365 Business
// Central. Every customer adapter (Falabella today) converts its own files
// into []Line and this package writes them.
//
// # Template
//
// The output is a copy of a template workbook (templates/standard.xlsx):
//
//	row 1  totals  ("TOTALES" plus SUM formulas; left untouched)
//	row 2  headers
//	row 3  formatting prototype: its cell styles (number formats, fonts...)
//	       are copied to every written row; the template keeps it empty
//
// Columns are found by their header text (case and surrounding spaces are
// ignored), never by position. Because the schema is not frozen yet this
// means:
//
//   - columns can be reordered or added to the template without code
//     changes; headers the code does not know are left empty;
//   - a header the code knows but the template lacks is skipped, unless the
//     column is required, in which case LoadTemplate fails right away.
//
// To add a field: add it to Line, add its header constant and one entry to
// [columns], and add the header to the template.
package standard

import (
	"time"
)

// Header names of the standard file.
const (
	HeaderTaxID          = "RUC"
	HeaderCustomer       = "CLIENTE"
	HeaderOrderNumber    = "N° ORDEN"
	HeaderOrderDate      = "FECHA ORDEN"
	HeaderDeliveryDate   = "FECHA ENTREGA"
	HeaderPaymentTerms   = "CONDICIÓN PAGO"
	HeaderBrand          = "MARCA"
	HeaderProductCode    = "CÓD. PRODUCTO"
	HeaderBarcode        = "CÓD. BARRAS"
	HeaderDescription    = "DESCRIPCIÓN"
	HeaderSize           = "TAMAÑO"
	HeaderUnitsPerBox    = "UXC"
	HeaderQuantity       = "CANTIDAD"
	HeaderUnitPrice      = "PRECIO UNIT."
	HeaderDiscount1      = "DESCUENTO 1 %"
	HeaderDiscount2      = "DESCUENTO 2 %"
	HeaderSubtotal       = "SUBTOTAL"
	HeaderVAT            = "IVA %"
	HeaderTotal          = "TOTAL"
	HeaderCategory       = "CATEGORÍA"
	HeaderHealthRegistry = "REG. SANITARIO"
	HeaderSourceDocument = "DOCUMENTO ORIGEN"
	HeaderItemNo         = "No"
	HeaderParentCode     = "CODIGO_PADRE"
	HeaderObservation    = "OBSERVACION"
	HeaderNaosPrice      = "Precio Naos"
	HeaderNaosDiscount   = "Descuento Naos"
	HeaderNaosSubtotal   = "Subtotal Naos"
	HeaderDifference     = "Diferencia"
	HeaderError          = "Error"

	// Added for Falabella; appended at the end of the template so existing
	// column positions do not move.
	HeaderCurrency = "MONEDA"
)

// DateLayout is how dates are written: as text, like the existing standard files.
const DateLayout = "02/01/2006"

// Line is one row of the standard file. The standard file is the input
// loaded into Dynamics, so a Line only holds order data; process data such
// as the billing date lives in order.Record. Zero values are written as
// empty cells, except numeric fields that are always meaningful
// (quantities, prices, discounts, subtotal, total).
type Line struct {
	// Customer.
	TaxID        string // RUC / NIT of the customer
	CustomerName string
	PaymentTerms string

	// Order.
	OrderNumber  string
	OrderDate    time.Time
	DeliveryDate time.Time // last delivery day
	Currency     string

	// Product.
	Brand          string
	ProductCode    string // NAOS internal code
	Barcode        string // EAN / UPC
	Description    string
	Size           string
	UnitsPerBox    int
	Category       string
	HealthRegistry string

	// Amounts.
	Quantity     int
	UnitPrice    float64
	Discount1Pct float64
	Discount2Pct float64
	Subtotal     float64
	VATPct       *float64 // nil = unknown, written as an empty cell
	Total        float64

	SourceDocument string

	// Catalog is the match against the NAOS product master; nil when the
	// product was not found (or no master is configured).
	Catalog *CatalogMatch

	Observation string
	Error       string
}

// CatalogMatch is the NAOS master data for a line.
type CatalogMatch struct {
	ItemNo      string  // "No": item number in Business Central
	ParentCode  string  // "CODIGO_PADRE"
	Price       float64 // "Precio Naos"
	DiscountPct float64 // "Descuento Naos"
	Subtotal    float64 // "Subtotal Naos"
	Difference  float64 // "Diferencia": document subtotal - NAOS subtotal
}

// column maps a header to the value written for a line.
type column struct {
	header   string
	required bool
	value    func(Line) any // nil or "" leaves the cell empty
}

// columns is the schema known by the code. Order does not matter.
// Required columns are the minimum needed to load an order.
var columns = []column{
	{header: HeaderTaxID, value: func(l Line) any { return l.TaxID }},
	{header: HeaderCustomer, value: func(l Line) any { return l.CustomerName }},
	{header: HeaderOrderNumber, required: true, value: func(l Line) any { return l.OrderNumber }},
	{header: HeaderOrderDate, value: func(l Line) any { return date(l.OrderDate) }},
	{header: HeaderDeliveryDate, value: func(l Line) any { return date(l.DeliveryDate) }},
	{header: HeaderPaymentTerms, value: func(l Line) any { return l.PaymentTerms }},
	{header: HeaderBrand, value: func(l Line) any { return l.Brand }},
	{header: HeaderProductCode, required: true, value: func(l Line) any { return l.ProductCode }},
	{header: HeaderBarcode, value: func(l Line) any { return l.Barcode }},
	{header: HeaderDescription, value: func(l Line) any { return l.Description }},
	{header: HeaderSize, value: func(l Line) any { return l.Size }},
	{header: HeaderUnitsPerBox, value: func(l Line) any { return positive(l.UnitsPerBox) }},
	{header: HeaderQuantity, required: true, value: func(l Line) any { return l.Quantity }},
	{header: HeaderUnitPrice, required: true, value: func(l Line) any { return l.UnitPrice }},
	{header: HeaderDiscount1, value: func(l Line) any { return l.Discount1Pct }},
	{header: HeaderDiscount2, value: func(l Line) any { return l.Discount2Pct }},
	{header: HeaderSubtotal, value: func(l Line) any { return l.Subtotal }},
	{header: HeaderVAT, value: func(l Line) any { return deref(l.VATPct) }},
	{header: HeaderTotal, value: func(l Line) any { return l.Total }},
	{header: HeaderCategory, value: func(l Line) any { return l.Category }},
	{header: HeaderHealthRegistry, value: func(l Line) any { return l.HealthRegistry }},
	{header: HeaderSourceDocument, value: func(l Line) any { return l.SourceDocument }},
	{header: HeaderItemNo, value: catalog(func(c *CatalogMatch) any { return c.ItemNo })},
	{header: HeaderParentCode, value: catalog(func(c *CatalogMatch) any { return c.ParentCode })},
	{header: HeaderObservation, value: func(l Line) any { return l.Observation }},
	{header: HeaderNaosPrice, value: catalog(func(c *CatalogMatch) any { return c.Price })},
	{header: HeaderNaosDiscount, value: catalog(func(c *CatalogMatch) any { return c.DiscountPct })},
	{header: HeaderNaosSubtotal, value: catalog(func(c *CatalogMatch) any { return c.Subtotal })},
	{header: HeaderDifference, value: catalog(func(c *CatalogMatch) any { return c.Difference })},
	{header: HeaderError, value: func(l Line) any { return l.Error }},
	{header: HeaderCurrency, value: func(l Line) any { return l.Currency }},
}

func date(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(DateLayout)
}

func positive(n int) any {
	if n <= 0 {
		return nil
	}
	return n
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func catalog(fn func(*CatalogMatch) any) func(Line) any {
	return func(l Line) any {
		if l.Catalog == nil {
			return nil
		}
		return fn(l.Catalog)
	}
}
