package payment

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"link-bot/internal/database"
)

type balanceTestPurchases struct {
	purchase  *database.Purchase
	createErr error
}

func (p *balanceTestPurchases) Create(_ context.Context, purchase *database.Purchase) (int64, error) {
	if p.createErr != nil {
		return 0, p.createErr
	}
	purchase.ID = 91
	p.purchase = purchase
	return purchase.ID, nil
}
func (p *balanceTestPurchases) UpdateFields(_ context.Context, _ int64, fields map[string]interface{}) error {
	p.purchase.Status = fields["status"].(database.PurchaseStatus)
	return nil
}
func (p *balanceTestPurchases) FindById(context.Context, int64) (*database.Purchase, error) {
	return p.purchase, nil
}

type balanceTestWallet struct {
	balance    int64
	customers  []int64
	deltas     []int64
	references []string
}

func (w *balanceTestWallet) Apply(_ context.Context, customer, delta int64, _, reference, _ string) (int64, bool, error) {
	if w.balance+delta < 0 {
		return w.balance, false, database.ErrInsufficientBalance
	}
	w.balance += delta
	w.customers = append(w.customers, customer)
	w.deltas = append(w.deltas, delta)
	w.references = append(w.references, reference)
	return w.balance, true, nil
}

func balanceGiftOptions(recipient *int64) CreatePurchaseOptions {
	token := uuid.New()
	return CreatePurchaseOptions{PurchaseKind: database.PurchaseKindGift, GiftRecipientUsername: " friend_user ", GiftRecipientCustomerID: recipient, GiftToken: &token, PlanID: "gift-plan", AgreementAccepted: true}
}

func TestBalanceGiftPersistsRecipientAndChargesSender(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "known recipient", false: "gift awaiting first login"}[known], func(t *testing.T) {
			var recipient *int64
			id := int64(22)
			if known {
				recipient = &id
			}
			options := balanceGiftOptions(recipient)
			promoID, traffic, devices := int64(3), int64(10<<30), 5
			options.PromoCodeID, options.PromoCodeCode, options.PromoDiscountPercent = &promoID, "GIFT25", 25
			options.DurationDays, options.TrafficLimitBytes, options.DeviceLimitCount = 30, &traffic, &devices
			store, wallet := &balanceTestPurchases{}, &balanceTestWallet{balance: 30000}
			url, purchaseID, err := settleBalancePurchase(context.Background(), store, wallet, 149.25, 1, &database.Customer{ID: 11}, options, func(_ context.Context, id int64) error {
				p := store.purchase
				if p.ID != id || p.PurchaseKind != database.PurchaseKindGift || p.GiftToken == nil || *p.GiftToken != *options.GiftToken || p.GiftRecipientUsername == nil || *p.GiftRecipientUsername != "friend_user" || p.GiftRecipientCustomerID != recipient {
					t.Fatalf("gift metadata lost: %+v", p)
				}
				if p.CustomerID != 11 || p.SubscriptionID != nil || wallet.balance != 15075 {
					t.Fatalf("sender must be charged before gift delivery: %+v, balance=%d", p, wallet.balance)
				}
				if p.PromoCodeID != &promoID || p.PromoCodeSnapshot == nil || *p.PromoCodeSnapshot != "GIFT25" || *p.PromoCodeDiscountPercent != 25 || *p.PlanID != "gift-plan" || p.Days != 30 || p.TrafficLimitBytes != &traffic || p.DeviceLimitCount != &devices {
					t.Fatalf("gift tariff/promo terms lost: %+v", p)
				}
				p.Status = database.PurchaseStatusPaid
				return nil
			})
			if err != nil || url != "" || purchaseID != 91 {
				t.Fatalf("settlement result: %q %d %v", url, purchaseID, err)
			}
			if len(wallet.deltas) != 1 || wallet.deltas[0] != -14925 || wallet.customers[0] != 11 || wallet.references[0] != "balance-purchase:91" {
				t.Fatalf("unexpected ledger: %+v", wallet)
			}
		})
	}
}

func TestBalanceGiftInsufficientFundsNeverDelivers(t *testing.T) {
	store, wallet := &balanceTestPurchases{}, &balanceTestWallet{balance: 1000}
	_, _, err := settleBalancePurchase(context.Background(), store, wallet, 199, 1, &database.Customer{ID: 11}, balanceGiftOptions(nil), func(context.Context, int64) error { t.Fatal("must not deliver an unpaid gift"); return nil })
	if !errors.Is(err, database.ErrInsufficientBalance) || wallet.balance != 1000 || len(wallet.deltas) != 0 || store.purchase.Status != database.PurchaseStatusCancel {
		t.Fatalf("unexpected insufficient funds result: %v %+v %+v", err, wallet, store.purchase)
	}
}

func TestBalanceGiftRefundsOnlyUnpaidDeliveryFailures(t *testing.T) {
	for _, paid := range []bool{false, true} {
		t.Run(map[bool]string{false: "delivery failed", true: "already paid"}[paid], func(t *testing.T) {
			store, wallet := &balanceTestPurchases{}, &balanceTestWallet{balance: 30000}
			failure := errors.New("delivery failed")
			_, _, err := settleBalancePurchase(context.Background(), store, wallet, 199, 1, &database.Customer{ID: 11}, balanceGiftOptions(nil), func(context.Context, int64) error {
				if paid {
					store.purchase.Status = database.PurchaseStatusPaid
				}
				return failure
			})
			if !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if paid {
				if wallet.balance != 10100 || len(wallet.deltas) != 1 || store.purchase.Status != database.PurchaseStatusPaid {
					t.Fatalf("paid gift must not be refunded: %+v", wallet)
				}
			} else if wallet.balance != 30000 || len(wallet.deltas) != 2 || wallet.deltas[1] != 19900 || wallet.references[1] != "balance-purchase-refund:91" || wallet.customers[1] != 11 || store.purchase.Status != database.PurchaseStatusCancel {
				t.Fatalf("refund must restore sender balance and cancel order: %+v", wallet)
			}
		})
	}
}

func TestInvalidBalanceGiftCannotCreateOrDebit(t *testing.T) {
	self := int64(11)
	missingToken := balanceGiftOptions(nil)
	missingToken.GiftToken = nil
	missingUsername := balanceGiftOptions(nil)
	missingUsername.GiftRecipientUsername = " "
	for _, options := range []CreatePurchaseOptions{missingToken, missingUsername, balanceGiftOptions(&self)} {
		store, wallet := &balanceTestPurchases{}, &balanceTestWallet{balance: 30000}
		_, _, err := settleBalancePurchase(context.Background(), store, wallet, 199, 1, &database.Customer{ID: 11}, options, nil)
		if err == nil || store.purchase != nil || len(wallet.deltas) != 0 {
			t.Fatalf("invalid gift accepted: %v", err)
		}
	}
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1), 0.001} {
		store, wallet := &balanceTestPurchases{}, &balanceTestWallet{balance: 30000}
		_, _, err := settleBalancePurchase(context.Background(), store, wallet, amount, 1, &database.Customer{ID: 11}, balanceGiftOptions(nil), nil)
		if err == nil || store.purchase != nil || len(wallet.deltas) != 0 {
			t.Fatalf("invalid amount accepted: %v", amount)
		}
	}
}

func TestBalanceStoreFailureDoesNotDebitAndSubscriptionStillWorks(t *testing.T) {
	store, wallet := &balanceTestPurchases{createErr: errors.New("database unavailable")}, &balanceTestWallet{balance: 30000}
	_, _, err := settleBalancePurchase(context.Background(), store, wallet, 199, 1, &database.Customer{ID: 11}, balanceGiftOptions(nil), nil)
	if err == nil || len(wallet.deltas) != 0 {
		t.Fatal("must not debit when order was not saved")
	}
	store = &balanceTestPurchases{}
	_, _, err = settleBalancePurchase(context.Background(), store, wallet, 199, 1, &database.Customer{ID: 11}, CreatePurchaseOptions{PurchaseKind: database.PurchaseKindSubscription}, func(context.Context, int64) error { store.purchase.Status = database.PurchaseStatusPaid; return nil })
	if err != nil || wallet.balance != 10100 || store.purchase.GiftToken != nil {
		t.Fatalf("ordinary balance purchase changed: %v %+v", err, store.purchase)
	}
}
