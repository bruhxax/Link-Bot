package database

import (
	"strings"
	"testing"
)

func TestCustomerColumnDefinitionsStayInSync(t *testing.T) {
	if got, want := len(customerScanDestinations(&Customer{})), len(customerSelectColumns); got != want {
		t.Fatalf("customer scanner has %d destinations for %d selected columns", got, want)
	}

	clause := customerReturningClause()
	for _, column := range customerSelectColumns {
		if !strings.Contains(clause, column) {
			t.Fatalf("customer RETURNING clause is missing %q", column)
		}
	}
}
