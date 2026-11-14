package falabella

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("02-01-2006", s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestBillingDate documents the month rule with the examples from the meeting.
func TestBillingDate(t *testing.T) {
	tests := []struct {
		name               string
		issue, deliveryEnd string
		want               string
	}{
		{"delivery within the issue month: bill on issue date", "20-10-2026", "27-10-2026", "20-10-2026"},
		{"typical order", "03-11-2026", "11-11-2026", "03-11-2026"},
		{"issued at month end, delivered next month: bill on the 1st", "29-10-2026", "06-11-2026", "01-11-2026"},
		{"delivery ends two months later: 1st of that month", "30-10-2026", "05-12-2026", "01-12-2026"},
		{"year change", "30-12-2026", "05-01-2027", "01-01-2027"},
		{"same month number, different year", "15-11-2026", "15-11-2027", "01-11-2027"},
		{"delivery end before issue (bad data): issue date", "10-11-2026", "01-11-2026", "10-11-2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BillingDate(d(tt.issue), d(tt.deliveryEnd))
			if !got.Equal(d(tt.want)) {
				t.Errorf("BillingDate(%s, %s) = %s, want %s", tt.issue, tt.deliveryEnd, got.Format("02-01-2006"), tt.want)
			}
		})
	}
}

// TestProductCode documents the VIN rule: always the last 6 digits.
func TestProductCode(t *testing.T) {
	tests := []struct {
		vin     string
		want    string
		wantErr bool
	}{
		{vin: "520846", want: "520846"},          // usual case
		{vin: "3701129807729", want: "807729"},   // full EAN sent in the VIN
		{vin: " 3701129813638 ", want: "813638"}, // spaces are ignored
		{vin: "7050021", want: "050021"},         // 7 digits: still the last 6
		{vin: "12345", wantErr: true},            // too short
		{vin: "", wantErr: true},                 // missing
		{vin: "ABC123456", wantErr: true},        // not numeric
	}
	for _, tt := range tests {
		got, err := ProductCode(tt.vin)
		if (err != nil) != tt.wantErr {
			t.Errorf("ProductCode(%q) error = %v, wantErr %v", tt.vin, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ProductCode(%q) = %q, want %q", tt.vin, got, tt.want)
		}
	}
}
