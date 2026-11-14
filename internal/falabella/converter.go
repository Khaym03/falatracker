package falabella

import (
	"fmt"
	"path/filepath"

	"github.com/Khaym03/falatracker/internal/order"
	"github.com/Khaym03/falatracker/internal/standard"
)

// Converter turns downloaded reports into a standard file. It is what the
// tracker calls after each download and what `falatracker convert` uses.
type Converter struct {
	template *standard.Template
	customer Customer
	catalog  Catalog
}

// NewConverter returns a Converter that writes files based on template.
func NewConverter(template *standard.Template, customer Customer, catalog Catalog) *Converter {
	return &Converter{template: template, customer: customer, catalog: catalog}
}

// Convert writes the standard file next to the products report (see
// OutputName) and returns the record to store. The record has no status or
// timestamps; the caller sets them.
func (c *Converter) Convert(files order.Files) (order.Record, error) {
	return c.ConvertTo(files, "")
}

// ConvertTo is Convert with an explicit output path; an empty outPath means
// the default location. files.Distribution may be empty.
func (c *Converter) ConvertTo(files order.Files, outPath string) (order.Record, error) {
	o, err := ReadProducts(files.Products)
	if err != nil {
		return order.Record{}, err
	}

	var dist []Allocation
	if files.Distribution != "" {
		if dist, err = ReadDistribution(files.Distribution); err != nil {
			return order.Record{}, err
		}
	}

	res := Convert(o, dist, filepath.Base(files.Products), c.customer, c.catalog)

	if outPath == "" {
		outPath = filepath.Join(filepath.Dir(files.Products), OutputName(o))
	}
	if err := c.template.Write(outPath, res.Lines); err != nil {
		return order.Record{}, fmt.Errorf("write standard file for order %s: %w", o.Number, err)
	}

	files.Standard = outPath
	return order.Record{
		OrderNumber: o.Number,
		BillingDate: BillingDate(o.IssueDate, o.DeliveryEnd).Format(order.BillingDateLayout),
		Files:       files,
		Issues:      res.Issues,
	}, nil
}

// OutputName is the standard file name of an order, following the existing
// naming (e.g. OC_BIODERMA_4-11-2026.xlsx): OC_FALABELLA_<number>_<issue date>.xlsx
func OutputName(o Order) string {
	return fmt.Sprintf("OC_FALABELLA_%s_%s.xlsx", o.Number, o.IssueDate.Format("2-1-2006"))
}
