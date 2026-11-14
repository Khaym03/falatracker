package falabella

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// Headers of the Falabella reports used by the adapter.
const (
	colOrderNumber    = "Número OC"
	colBU             = "BU"
	colOrderStatus    = "Estado OC"
	colIssueDate      = "Fecha de emisión"
	colPaymentTerms   = "Forma de pago"
	colCurrency       = "Moneda"
	colGrossTotal     = "Total OC bruto"
	colDeliveryEnd    = "Fecha fin recepción"
	colUPC            = "UPC"
	colSKU            = "SKU"
	colDescription    = "Descripción"
	colVIN            = "VIN"
	colBrand          = "Marca"
	colClassification = "Descripción clasificación"
	colCost           = "Costo"
	colPurchased      = "Unidades compradas"
	colCancelled      = "Unidades canceladas"
	colLineCharges    = "Cargos línea"
	colLineDiscounts  = "Descuentos línea"
	colStoreID        = "Id de local"
	colStore          = "Local"
)

// dateLayout is the date format of the Falabella reports.
const dateLayout = "02-01-2006"

// Order is the content of a products-details report.
type Order struct {
	Number       string
	BU           string // e.g. FAL-CO
	Status       string // "Estado OC", e.g. Cursado
	IssueDate    time.Time
	DeliveryEnd  time.Time // "Fecha fin recepción": last delivery day
	PaymentTerms string
	Currency     string
	GrossTotal   float64 // "Total OC bruto"
	Products     []Product
}

// Product is one row of a products-details report.
type Product struct {
	UPC            string
	SKU            string // Falabella's own code
	Description    string
	VIN            string // NAOS code, sometimes the full EAN
	Brand          string
	Classification string
	Cost           float64 // unit cost paid by Falabella = our unit price
	Purchased      int
	Cancelled      int
	LineCharges    float64
	LineDiscounts  float64
}

// Allocation is one row of a distributions-details report: units of a
// product for one store.
type Allocation struct {
	SKU     string
	VIN     string
	StoreID string
	Store   string
	Units   int
}

// ReadProducts reads a products-details report.
func ReadProducts(path string) (Order, error) {
	t, err := readTable(path)
	if err != nil {
		return Order{}, err
	}
	if err := t.require(colOrderNumber, colIssueDate, colDeliveryEnd, colVIN, colCost, colPurchased); err != nil {
		return Order{}, err
	}
	if len(t.rows) == 0 {
		return Order{}, fmt.Errorf("%s: no products", path)
	}

	// Order data is repeated on every row; take it from the first one.
	first := t.rows[0]
	var o Order
	var errs []error
	o.Number = t.get(first, colOrderNumber)
	o.BU = t.get(first, colBU)
	o.Status = t.get(first, colOrderStatus)
	o.PaymentTerms = t.get(first, colPaymentTerms)
	o.Currency = t.get(first, colCurrency)
	o.IssueDate, err = parseDate(t.get(first, colIssueDate), colIssueDate)
	errs = append(errs, err)
	o.DeliveryEnd, err = parseDate(t.get(first, colDeliveryEnd), colDeliveryEnd)
	errs = append(errs, err)
	o.GrossTotal, err = parseFloat(t.get(first, colGrossTotal), colGrossTotal, true)
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return Order{}, fmt.Errorf("%s: %w", path, err)
	}

	for i, r := range t.rows {
		if n := t.get(r, colOrderNumber); n != o.Number {
			return Order{}, fmt.Errorf("%s row %d: order %q, expected %q", path, i+2, n, o.Number)
		}
		p, err := t.product(r)
		if err != nil {
			return Order{}, fmt.Errorf("%s row %d: %w", path, i+2, err)
		}
		o.Products = append(o.Products, p)
	}
	return o, nil
}

func (t *table) product(r []string) (Product, error) {
	p := Product{
		UPC:            t.get(r, colUPC),
		SKU:            t.get(r, colSKU),
		Description:    t.get(r, colDescription),
		VIN:            t.get(r, colVIN),
		Brand:          t.get(r, colBrand),
		Classification: t.get(r, colClassification),
	}
	var err error
	var errs []error
	p.Cost, err = parseFloat(t.get(r, colCost), colCost, false)
	errs = append(errs, err)
	p.Purchased, err = parseInt(t.get(r, colPurchased), colPurchased, false)
	errs = append(errs, err)
	p.Cancelled, err = parseInt(t.get(r, colCancelled), colCancelled, true)
	errs = append(errs, err)
	p.LineCharges, err = parseFloat(t.get(r, colLineCharges), colLineCharges, true)
	errs = append(errs, err)
	p.LineDiscounts, err = parseFloat(t.get(r, colLineDiscounts), colLineDiscounts, true)
	errs = append(errs, err)
	return p, errors.Join(errs...)
}

// ReadDistribution reads a distributions-details report.
func ReadDistribution(path string) ([]Allocation, error) {
	t, err := readTable(path)
	if err != nil {
		return nil, err
	}
	if err := t.require(colSKU, colPurchased); err != nil {
		return nil, err
	}

	out := make([]Allocation, 0, len(t.rows))
	for i, r := range t.rows {
		units, err := parseInt(t.get(r, colPurchased), colPurchased, false)
		if err != nil {
			return nil, fmt.Errorf("%s row %d: %w", path, i+2, err)
		}
		out = append(out, Allocation{
			SKU:     t.get(r, colSKU),
			VIN:     t.get(r, colVIN),
			StoreID: t.get(r, colStoreID),
			Store:   t.get(r, colStore),
			Units:   units,
		})
	}
	return out, nil
}

// table is the first sheet of a report: a header row followed by data rows.
type table struct {
	path string
	cols map[string]int // header -> index; the first one wins on duplicates
	rows [][]string
}

func readTable(path string) (*table, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: empty file", path)
	}

	t := &table{path: path, cols: make(map[string]int)}
	for i, h := range rows[0] {
		h = strings.TrimSpace(h)
		if _, dup := t.cols[h]; !dup && h != "" {
			t.cols[h] = i
		}
	}
	for _, r := range rows[1:] {
		if !blank(r) {
			t.rows = append(t.rows, r)
		}
	}
	return t, nil
}

func (t *table) require(headers ...string) error {
	var missing []string
	for _, h := range headers {
		if _, ok := t.cols[h]; !ok {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: missing column(s): %s", t.path, strings.Join(missing, ", "))
	}
	return nil
}

// get returns the trimmed cell of row under header, or "" if absent.
func (t *table) get(row []string, header string) string {
	i, ok := t.cols[header]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func blank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func parseDate(s, col string) (time.Time, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: invalid date %q (want dd-mm-yyyy)", col, s)
	}
	return t, nil
}

// parseFloat parses plain numbers as Falabella sends them ("125000",
// "12.5"). An empty value is 0 when optional.
func parseFloat(s, col string, optional bool) (float64, error) {
	if s == "" && optional {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid number %q", col, s)
	}
	return v, nil
}

func parseInt(s, col string, optional bool) (int, error) {
	if s == "" && optional {
		return 0, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", col, s)
	}
	return v, nil
}
