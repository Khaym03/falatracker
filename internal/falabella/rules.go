package falabella

import (
	"fmt"
	"strings"
	"time"
)

// BillingDate applies the month rule agreed with NAOS (meeting 2026-10-30):
//
//   - if the delivery window ends in the same month as the order was issued,
//     the order is billed on its issue date;
//   - if it ends in a later month (orders issued at the end of a month), the
//     order waits and is billed on the 1st of the month the delivery ends.
//
// Example: issued 29/10, delivery until 06/11 -> billed 01/11.
//
// Business days are deliberately ignored for now. A delivery end before the
// issue date (bad data) falls back to the issue date.
func BillingDate(issue, deliveryEnd time.Time) time.Time {
	sameMonth := issue.Year() == deliveryEnd.Year() && issue.Month() == deliveryEnd.Month()
	if sameMonth || deliveryEnd.Before(issue) {
		return issue
	}
	return time.Date(deliveryEnd.Year(), deliveryEnd.Month(), 1, 0, 0, 0, 0, issue.Location())
}

// ProductCode returns the NAOS product code of a Falabella VIN: its last 6
// digits. Falabella usually sends the 6-digit code, but sometimes the full
// 13-digit EAN (e.g. "3701129807729" -> "807729"); "Modelo" has the same
// problem, which is why the meeting chose the VIN.
func ProductCode(vin string) (string, error) {
	v := strings.TrimSpace(vin)
	if len(v) < 6 {
		return "", fmt.Errorf("VIN %q tiene menos de 6 dígitos", vin)
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("VIN %q no es numérico", vin)
		}
	}
	return v[len(v)-6:], nil
}
