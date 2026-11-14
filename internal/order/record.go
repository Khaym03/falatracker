package order

import "time"

// Status is the state of an order that falatracker already handled.
//
// Lifecycle:
//
//	(not in store) --download+convert--> ready --PAD loads it into Dynamics--> loaded
//
// An order whose download or conversion fails is not stored, so it is
// retried in the next cycle.
type Status string

const (
	// StatusReady means the reports were downloaded and converted to the
	// standard file. PAD decides when to load it (see Record.BillingDate).
	StatusReady Status = "ready"

	// StatusLoaded means PAD confirmed (POST /orders/{number}/loaded) that the
	// order was loaded into Dynamics.
	StatusLoaded Status = "loaded"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	return s == StatusReady || s == StatusLoaded
}

// Files are the paths of the files kept for an order.
type Files struct {
	Products     string `json:"products"`     // Falabella products-details report
	Distribution string `json:"distribution"` // Falabella distributions-details report (attached to the invoice e-mail)
	Standard     string `json:"standard"`     // generated standard Excel, the file loaded into Dynamics
}

// Record is what the store keeps about a handled order. It is also the JSON
// that PAD receives from GET /orders.
type Record struct {
	OrderNumber string `json:"order_number"`
	Status      Status `json:"status,omitempty"`

	// BillingDate (YYYY-MM-DD) is when the order should be loaded and
	// billed, after applying the month rule. PAD should not load it before
	// that date.
	BillingDate string `json:"billing_date"`

	Files Files `json:"files"`

	// Issues are order-level problems found during the conversion (totals
	// that do not match, placeholder customer data, ...). Line-level problems
	// are in the "Error" column of the standard file.
	Issues []string `json:"issues,omitempty"`

	ProcessedAt time.Time  `json:"processed_at,omitzero"`
	LoadedAt    *time.Time `json:"loaded_at,omitempty"`
}

// BillingDateLayout is the format of Record.BillingDate.
const BillingDateLayout = "2006-01-02"
