package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Khaym03/falatracker/internal/falabella"
	"github.com/Khaym03/falatracker/internal/order"
	"github.com/Khaym03/falatracker/internal/standard"
)

// conversionOptions are the flags shared by the tracker and `convert`.
type conversionOptions struct {
	template      string
	customerTaxID string
	customerName  string
	paymentTerms  string
	vat           float64 // negative = unknown
}

func addConversionFlags(fs *pflag.FlagSet, o *conversionOptions) {
	placeholder := falabella.PlaceholderCustomer()
	fs.StringVar(&o.template, "template", "templates/standard.xlsx", "standard Excel template")
	fs.StringVar(&o.customerTaxID, "customer-tax-id", placeholder.TaxID, "Falabella NIT for the RUC column")
	fs.StringVar(&o.customerName, "customer-name", placeholder.Name, "customer name for the CLIENTE column")
	fs.StringVar(&o.paymentTerms, "payment-terms", placeholder.PaymentTerms, "payment terms when the report has none")
	fs.Float64Var(&o.vat, "vat", -1, "VAT % for the IVA column; negative leaves it empty (unknown)")
}

// converter loads the template (failing fast if it is invalid) and builds
// the Falabella converter.
func (o conversionOptions) converter() (*falabella.Converter, error) {
	t, err := standard.LoadTemplate(o.template)
	if err != nil {
		return nil, err
	}
	customer := falabella.Customer{
		TaxID:        o.customerTaxID,
		Name:         o.customerName,
		PaymentTerms: o.paymentTerms,
	}
	if o.vat >= 0 {
		vat := o.vat
		customer.VATPct = &vat
	}
	return falabella.NewConverter(t, customer, falabella.NoCatalog{}), nil
}

// newConvertCmd converts already downloaded reports without opening the
// browser: useful to re-process an order, to try template changes and to
// check a conversion by hand.
func newConvertCmd() *cobra.Command {
	var (
		conv                        conversionOptions
		products, distribution, out string
	)

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert downloaded Falabella reports into the standard Excel (no browser)",
		Long: `Convert reads a products-details report (and optionally its distributions-details
report), writes the standard Excel file and prints the resulting order record as JSON,
including the billing date and any issues found.

It does not touch the order store: it is meant for re-processing and manual checks.`,
		Example: `  falatracker convert --products oc-1000001-products-details.xlsx \
    --distribution oc-1000001-distributions-details.xlsx -o out.xlsx`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := conv.converter()
			if err != nil {
				return err
			}
			rec, err := c.ConvertTo(order.Files{Products: products, Distribution: distribution}, out)
			if err != nil {
				return fmt.Errorf("convert: %w", err)
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(rec)
		},
	}

	f := cmd.Flags()
	f.StringVar(&products, "products", "", "products-details report (required)")
	f.StringVar(&distribution, "distribution", "", "distributions-details report (enables the units check)")
	f.StringVarP(&out, "output", "o", "", "output file (default: OC_FALABELLA_<order>_<date>.xlsx next to the products report)")
	addConversionFlags(f, &conv)
	_ = cmd.MarkFlagRequired("products")
	return cmd
}
