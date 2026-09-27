package miniapp

import "testing"

func TestAdminBalanceTransaction(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		action string
		cents  int64
		kind   string
		valid  bool
	}{
		{"credit", 50, "credit", 5000, "admin_credit", true},
		{"legacy credit", 50, "", 5000, "admin_credit", true},
		{"debit", 50, "debit", -5000, "admin_debit", true},
		{"zero", 0, "debit", 0, "", false},
		{"too large", 1000001, "credit", 0, "", false},
		{"unknown action", 50, "refund", 0, "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cents, kind, _, err := adminBalanceTransaction(test.amount, test.action)
			if (err == nil) != test.valid || cents != test.cents || kind != test.kind {
				t.Fatalf("adminBalanceTransaction(%d, %q) = (%d, %q, %v)", test.amount, test.action, cents, kind, err)
			}
		})
	}
}
