package falabella

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Khaym03/falatracker/internal/standard"
)

// Pending is the value used for customer data that is not known yet. It is
// meant to be obvious in the generated file so it is never loaded by mistake.
const Pending = "PENDIENTE"

// tolerance is the largest difference between two amounts that is still
// considered equal (same threshold as Core.py; revisit for COP).
const tolerance = 0.10

// Customer is the Falabella data that does not come in the reports.
//
// Extension point: today it is filled from CLI flags with Pending values;
// when the real NIT, payment terms and VAT are known, pass them as flags (no
// code change needed).
type Customer struct {
	TaxID        string
	Name         string
	PaymentTerms string
	VATPct       *float64 // nil = unknown
}

// PlaceholderCustomer is the Customer used until the real data is available.
func PlaceholderCustomer() Customer {
	return Customer{TaxID: Pending, Name: "FALABELLA (" + Pending + ")", PaymentTerms: Pending}
}

// missing lists the customer fields that are still placeholders.
func (c Customer) missing() []string {
	var m []string
	if c.TaxID == "" || c.TaxID == Pending {
		m = append(m, "RUC/NIT")
	}
	if c.PaymentTerms == "" || c.PaymentTerms == Pending {
		m = append(m, "condición de pago")
	}
	if c.VATPct == nil {
		m = append(m, "IVA")
	}
	return m
}

// Item is a product of the NAOS master and price list.
type Item struct {
	ItemNo      string  // item number in Business Central ("No")
	ParentCode  string  // "CODIGO_PADRE"
	Price       float64 // list price before discount
	DiscountPct float64
}

// Catalog finds products in the NAOS master by product code.
//
// Extension point: implement it over the Colombia master/price list when
// available (for instance reading TEST_MAESTRO_PRODUCTOS.xlsx and
// Precios.xlsx like Core.py). Until then NoCatalog is used.
type Catalog interface {
	Lookup(productCode string) (Item, bool)
}

// NoCatalog is a Catalog without products: every line is reported as not
// found, exactly as Core.py reports an unknown product.
type NoCatalog struct{}

// Lookup implements [Catalog].
func (NoCatalog) Lookup(string) (Item, bool) { return Item{}, false }

// Result of a conversion.
type Result struct {
	Lines  []standard.Line
	Issues []string // order-level problems
}

// Convert maps an order to standard lines and runs the checks. It never
// fails: problems end up in the Error/OBSERVACION columns or in Issues.
// dist may be nil when the distribution file is not available.
func Convert(o Order, dist []Allocation, sourceDocument string, customer Customer, catalog Catalog) Result {
	paymentTerms := o.PaymentTerms
	if paymentTerms == "" {
		paymentTerms = customer.PaymentTerms
	}

	var res Result
	var gross float64
	for _, p := range o.Products {
		line := standard.Line{
			TaxID:          customer.TaxID,
			CustomerName:   customer.Name,
			PaymentTerms:   paymentTerms,
			OrderNumber:    o.Number,
			OrderDate:      o.IssueDate,
			DeliveryDate:   o.DeliveryEnd,
			Currency:       o.Currency,
			Brand:          p.Brand,
			Barcode:        p.UPC,
			Description:    p.Description,
			Category:       p.Classification,
			Quantity:       p.Purchased,
			UnitPrice:      p.Cost,
			Subtotal:       round2(p.Cost * float64(p.Purchased)),
			VATPct:         customer.VATPct,
			SourceDocument: sourceDocument,
		}
		line.Total = line.Subtotal
		if customer.VATPct != nil {
			line.Total = round2(line.Subtotal * (1 + *customer.VATPct/100))
		}
		gross += line.Subtotal

		var obs, errs []string
		code, err := ProductCode(p.VIN)
		if err != nil {
			errs = append(errs, err.Error())
		}
		line.ProductCode = code

		if p.Cancelled > 0 {
			obs = append(obs, fmt.Sprintf("Unidades canceladas: %d", p.Cancelled))
		}
		if p.LineCharges != 0 || p.LineDiscounts != 0 {
			errs = append(errs, fmt.Sprintf("Cargos/descuentos de línea no soportados (cargos=%.2f, descuentos=%.2f)", p.LineCharges, p.LineDiscounts))
		}

		if code != "" {
			if item, ok := catalog.Lookup(code); ok {
				line.Catalog = match(item, line)
				if math.Abs(line.Catalog.Difference) > tolerance {
					errs = append(errs, fmt.Sprintf("Diferencia de subtotal: doc=%.2f vs naos=%.2f", line.Subtotal, line.Catalog.Subtotal))
				}
			} else {
				obs = append(obs, fmt.Sprintf("No encontrado en maestro (Búsqueda: %s)", code))
				errs = append(errs, "Sin código 'No' en la fila")
			}
		}

		line.Observation = strings.Join(obs, "; ")
		line.Error = strings.Join(errs, "; ")
		res.Lines = append(res.Lines, line)
	}

	if math.Abs(gross-o.GrossTotal) > tolerance {
		res.Issues = append(res.Issues, fmt.Sprintf("Suma de subtotales (%.2f) distinta al Total OC bruto (%.2f)", gross, o.GrossTotal))
	}
	if dist == nil {
		res.Issues = append(res.Issues, "Sin archivo de distribución: no se validaron unidades por tienda")
	} else {
		res.Issues = append(res.Issues, checkDistribution(o.Products, dist)...)
	}
	if m := customer.missing(); len(m) > 0 {
		res.Issues = append(res.Issues, "Datos de cliente pendientes: "+strings.Join(m, ", "))
	}
	return res
}

// match fills the NAOS columns for a line found in the catalog.
func match(item Item, line standard.Line) *standard.CatalogMatch {
	subtotal := round2(item.Price * (1 - item.DiscountPct/100) * float64(line.Quantity))
	return &standard.CatalogMatch{
		ItemNo:      item.ItemNo,
		ParentCode:  item.ParentCode,
		Price:       item.Price,
		DiscountPct: item.DiscountPct,
		Subtotal:    subtotal,
		Difference:  round2(line.Subtotal - subtotal),
	}
}

// checkDistribution verifies that, for every SKU, the units split across
// stores add up to the units purchased.
func checkDistribution(products []Product, dist []Allocation) []string {
	want := make(map[string]int)
	for _, p := range products {
		want[p.SKU] += p.Purchased
	}
	got := make(map[string]int)
	for _, a := range dist {
		got[a.SKU] += a.Units
	}

	skus := make(map[string]bool)
	for s := range want {
		skus[s] = true
	}
	for s := range got {
		skus[s] = true
	}
	sorted := make([]string, 0, len(skus))
	for s := range skus {
		sorted = append(sorted, s)
	}
	sort.Strings(sorted)

	var issues []string
	for _, s := range sorted {
		if want[s] != got[s] {
			issues = append(issues, fmt.Sprintf("SKU %s: %d unidades compradas vs %d en distribución", s, want[s], got[s]))
		}
	}
	return issues
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
