package database

import (
	"context"
	"reflect"
	"strings"
	"testing"

	sq "github.com/Masterminds/squirrel"
)

func TestBuildLatestActiveTributesQuery(t *testing.T) {
	customerIDs := []int64{10, 20}

	builder := buildLatestActiveTributesQuery(customerIDs).PlaceholderFormat(sq.Dollar)
	sql, args, err := builder.ToSql()
	if err != nil {
		t.Fatalf("ToSql() returned error: %v", err)
	}

	if !strings.Contains(sql, "created_at = (SELECT MAX(created_at)") {
		t.Fatalf("expected SQL to contain subquery selecting latest tribute, got: %s", sql)
	}

	if !strings.Contains(sql, "status <>") {
		t.Fatalf("expected SQL to exclude cancelled tributes, got: %s", sql)
	}

	expectedArgs := []interface{}{InvoiceTypeTribute, int64(10), int64(20), InvoiceTypeTribute, PurchaseStatusCancel}
	if !reflect.DeepEqual(args, expectedArgs) {
		t.Fatalf("unexpected args, want %v, got %v", expectedArgs, args)
	}
}

func TestFindLatestActiveTributesByCustomerIDsEmpty(t *testing.T) {
	repo := &PurchaseRepository{}

	result, err := repo.FindLatestActiveTributesByCustomerIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatalf("result should not be nil")
	}

	if len(*result) != 0 {
		t.Fatalf("expected empty result, got %d", len(*result))
	}
}
