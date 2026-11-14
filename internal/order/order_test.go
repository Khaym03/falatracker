package order

import (
	"testing"
)

func TestParsePercentage(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{in: "0%", want: 0},
		{in: "100%", want: 100},
		{in: "12,5%", want: 12.5},
		{in: " 7.25 % ", want: 7.25},
		{in: "50", want: 50},
		{in: "", wantErr: true},
		{in: "n/a", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePercentage(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePercentage(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParsePercentage(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func validCells() []string {
	return []string{
		" FALABELLA ", "4500123", "01/11/2026", "05/11/2026", "10/11/2026",
		"Directo", "CD", "$1.000", "10", "0%", " Pending ",
	}
}

func TestFromCells(t *testing.T) {
	cells := validCells()
	got, err := FromCells(cells)
	if err != nil {
		t.Fatalf("FromCells: %v", err)
	}

	want := PurchaseOrder{
		BU:                  "FALABELLA",
		OrderNumber:         "4500123",
		CreationDate:        "01/11/2026",
		DeliveryStartDate:   "05/11/2026",
		DeliveryEndDate:     "10/11/2026",
		DeliveryType:        "Directo",
		Delivery:            "CD",
		TotalAmount:         "$1.000",
		TotalQuantity:       "10",
		AgreementPercentage: 0,
		Status:              "Pending",
	}
	if got != want {
		t.Errorf("FromCells() = %+v, want %+v", got, want)
	}
	if cells[0] != " FALABELLA " {
		t.Error("FromCells must not modify its input")
	}
}

func TestFromCellsErrors(t *testing.T) {
	t.Run("too few cells", func(t *testing.T) {
		if _, err := FromCells([]string{"only", "two"}); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("bad percentage", func(t *testing.T) {
		cells := validCells()
		cells[9] = "abc"
		if _, err := FromCells(cells); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestNeedsReports(t *testing.T) {
	tests := []struct {
		name string
		o    PurchaseOrder
		want bool
	}{
		{"pending without agreement", PurchaseOrder{Status: "Pending", AgreementPercentage: 0}, true},
		{"canceled", PurchaseOrder{Status: "Canceled"}, false},
		{"canceled any case", PurchaseOrder{Status: "CANCELED"}, false},
		{"agreement in progress", PurchaseOrder{Status: "Pending", AgreementPercentage: 0.5}, false},
		{"fully agreed", PurchaseOrder{Status: "Pending", AgreementPercentage: 100}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.o.NeedsReports(); got != tt.want {
				t.Errorf("NeedsReports() = %v, want %v", got, tt.want)
			}
		})
	}
}
