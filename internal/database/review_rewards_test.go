package database

import "testing"

func TestPersonalReviewPromoOwnership(t *testing.T) {
	owner := int64(42)
	p := &PromoCode{OwnerCustomerID: &owner}
	if !p.AvailableToCustomer(owner) {
		t.Fatal("owner cannot use personal promo")
	}
	for _, other := range []int64{0, -1, 41, 43} {
		if p.AvailableToCustomer(other) {
			t.Fatalf("personal promo accessible by %d", other)
		}
	}
	p.OwnerCustomerID = nil
	if !p.AvailableToCustomer(0) || !p.AvailableToCustomer(43) {
		t.Fatal("public promos should remain public")
	}
}
