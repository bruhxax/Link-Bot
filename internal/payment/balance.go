package payment

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"link-bot/internal/database"
)

// Keep balance settlement identical for subscription, add-on and gift purchases.
// Repositories still own the transaction lock and idempotent ledger references.
type balancePurchaseStore interface {
	Create(context.Context, *database.Purchase) (int64, error)
	UpdateFields(context.Context, int64, map[string]interface{}) error
	FindById(context.Context, int64) (*database.Purchase, error)
}

type balanceWallet interface {
	Apply(context.Context, int64, int64, string, string, string) (int64, bool, error)
}

func settleBalancePurchase(ctx context.Context, purchases balancePurchaseStore, wallet balanceWallet, amount float64, months int, customer *database.Customer, options CreatePurchaseOptions, process func(context.Context, int64) error) (string, int64, error) {
	if customer == nil || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || math.Round(amount*100) < 1 {
		return "", 0, errors.New("invalid balance purchase")
	}
	if options.PurchaseKind == database.PurchaseKindGift {
		if options.GiftToken == nil || strings.TrimSpace(options.GiftRecipientUsername) == "" {
			return "", 0, errors.New("gift purchase metadata is incomplete")
		}
		if options.GiftRecipientCustomerID != nil && *options.GiftRecipientCustomerID == customer.ID {
			return "", 0, errors.New("a subscription gift cannot be delivered to its sender")
		}
	}
	amountCents := int64(math.Round(amount * 100))
	purchaseID, err := purchases.Create(ctx, &database.Purchase{
		InvoiceType:              database.InvoiceTypeBalance,
		Status:                   database.PurchaseStatusNew,
		Amount:                   amount,
		Currency:                 "RUB",
		CustomerID:               customer.ID,
		SubscriptionID:           options.SubscriptionID,
		Month:                    months,
		Days:                     options.DurationDays,
		PlanID:                   optionalTrimmedStringPointer(options.PlanID),
		TrafficLimitBytes:        options.TrafficLimitBytes,
		DeviceLimitCount:         options.DeviceLimitCount,
		AgreementAccepted:        options.AgreementAccepted,
		PurchaseKind:             options.PurchaseKind,
		ExtraDevices:             options.ExtraDevices,
		ExtraTrafficBytes:        options.ExtraTrafficBytes,
		ExtraTrafficUnlimited:    options.ExtraTrafficUnlimited,
		DeviceExpiresAt:          options.DeviceExpiresAt,
		IsFreePlan:               options.IsFreePlan,
		FreePlanOneTime:          options.FreePlanOneTime,
		GiftRecipientUsername:    optionalTrimmedStringPointer(options.GiftRecipientUsername),
		GiftRecipientCustomerID:  options.GiftRecipientCustomerID,
		GiftToken:                options.GiftToken,
		PromoCodeID:              options.PromoCodeID,
		PromoCodeSnapshot:        optionalTrimmedStringPointer(options.PromoCodeCode),
		PromoCodeDiscountPercent: optionalPositiveIntPointer(options.PromoDiscountPercent),
	})
	if err != nil {
		return "", 0, err
	}
	_, _, err = wallet.Apply(ctx, customer.ID, -amountCents, "purchase", fmt.Sprintf("balance-purchase:%d", purchaseID), "Оплата покупки с баланса")
	if err != nil {
		_ = purchases.UpdateFields(ctx, purchaseID, map[string]interface{}{"status": database.PurchaseStatusCancel})
		return "", purchaseID, err
	}
	if err := process(ctx, purchaseID); err != nil {
		current, findErr := purchases.FindById(ctx, purchaseID)
		if findErr == nil && current != nil && current.Status != database.PurchaseStatusPaid {
			_, _, _ = wallet.Apply(ctx, customer.ID, amountCents, "purchase_refund", fmt.Sprintf("balance-purchase-refund:%d", purchaseID), "Возврат за неисполненную покупку")
			_ = purchases.UpdateFields(ctx, purchaseID, map[string]interface{}{"status": database.PurchaseStatusCancel})
		}
		return "", purchaseID, err
	}
	return "", purchaseID, nil
}
