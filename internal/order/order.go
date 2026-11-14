// Package order holds the purchase order domain model and the rules that
// decide which orders need their reports downloaded. It has no dependency on
// the browser, so it can be tested in isolation.
package order

import (
	"fmt"
	"strconv"
	"strings"
)

// Columns is the number of cells a purchase order row has in the portal table.
const Columns = 11

// PurchaseOrder is a row of the purchase order history table.
type PurchaseOrder struct {
	BU                  string
	OrderNumber         string
	CreationDate        string
	DeliveryStartDate   string
	DeliveryEndDate     string
	DeliveryType        string
	Delivery            string
	TotalAmount         string
	TotalQuantity       string
	AgreementPercentage float64
	Status              string
}

// FromCells builds a PurchaseOrder from the text of a table row's cells, in
// the same order the portal shows them.
func FromCells(cells []string) (PurchaseOrder, error) {
	if len(cells) < Columns {
		return PurchaseOrder{}, fmt.Errorf("expected %d cells, got %d", Columns, len(cells))
	}

	cell := func(i int) string { return strings.TrimSpace(cells[i]) }

	pct, err := ParsePercentage(cell(9))
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("order %q: %w", cell(1), err)
	}

	return PurchaseOrder{
		BU:                  cell(0),
		OrderNumber:         cell(1),
		CreationDate:        cell(2),
		DeliveryStartDate:   cell(3),
		DeliveryEndDate:     cell(4),
		DeliveryType:        cell(5),
		Delivery:            cell(6),
		TotalAmount:         cell(7),
		TotalQuantity:       cell(8),
		AgreementPercentage: pct,
		Status:              cell(10),
	}, nil
}

// ParsePercentage parses portal values such as "0%", "12,5%" or "100".
func ParsePercentage(s string) (float64, error) {
	clean := strings.ReplaceAll(s, "%", "")
	clean = strings.ReplaceAll(clean, ",", ".")
	clean = strings.TrimSpace(clean)

	v, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid agreement percentage %q: %w", s, err)
	}
	return v, nil
}

// NeedsReports reports whether the order's reports should be downloaded:
// the order must not be canceled and must have no agreement progress yet.
func (o PurchaseOrder) NeedsReports() bool {
	if strings.EqualFold(o.Status, "canceled") {
		return false
	}
	if o.AgreementPercentage > 0 {
		return false
	}
	return true
}
