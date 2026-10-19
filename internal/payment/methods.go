package payment

import (
	"context"
	"fmt"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/paymentchannel"
	"link-bot/internal/runtimeconfig"
)

// AvailableMethods is shared by Telegram checkout and the Mini App.
func (s PaymentService) AvailableMethods(ctx context.Context, customer *database.Customer, settings runtimeconfig.Settings) (map[string]bool, error) {
	methods := map[string]bool{
		"sbp":    s.IsProviderEnabled(integrations.ProviderYooKassa),
		"card":   s.IsProviderEnabled(integrations.ProviderYooKassa),
		"crypto": s.IsProviderEnabled(integrations.ProviderCryptoPay),
		"stars":  settings.Features["stars"],
	}
	for _, provider := range integrations.SortedPaymentProviders() {
		if provider != integrations.ProviderYooKassa && provider != integrations.ProviderCryptoPay {
			methods[provider] = s.IsProviderEnabled(provider)
		}
	}
	methods["p2p"] = methods["p2p"] && s.IsProviderEnabled(integrations.ProviderNotificationBot)
	if customer == nil || customer.TelegramIDIsSynthetic {
		methods["stars"] = false
	}
	if settings.Referrals.BalancePaymentsEnabled && s.walletRepository != nil && customer != nil {
		balance, err := s.walletRepository.Balance(ctx, customer.ID)
		if err != nil {
			return nil, err
		}
		methods["balance"] = balance > 0
	}
	if methods["stars"] && config.RequirePaidPurchaseForStars() {
		purchase, err := s.purchaseRepository.FindSuccessfulPaidPurchaseByCustomer(ctx, customer.ID)
		if err != nil {
			return nil, err
		}
		methods["stars"] = purchase != nil
	}
	for id, variant := range settings.PaymentMethodVariants {
		methods[id] = methods[variant.Method]
	}
	return methods, nil
}

func InvoiceTypeForMethod(method string) (database.InvoiceType, error) {
	method = paymentchannel.Base(method)
	switch method {
	case "balance":
		return database.InvoiceTypeBalance, nil
	case "sbp", "card":
		return database.InvoiceTypeYookasa, nil
	case "stars":
		return database.InvoiceTypeTelegram, nil
	case "crypto":
		return database.InvoiceTypeCrypto, nil
	case "p2p":
		return database.InvoiceTypeP2P, nil
	default:
		if invoiceType, ok := invoiceTypeForIntegrationProvider(method); ok {
			return invoiceType, nil
		}
		return "", fmt.Errorf("unknown payment method: %s", method)
	}
}

func MethodForInvoiceType(invoiceType database.InvoiceType) string {
	switch invoiceType {
	case database.InvoiceTypeYookasa:
		return "card"
	case database.InvoiceTypeTelegram:
		return "stars"
	case database.InvoiceTypeCrypto:
		return "crypto"
	case database.InvoiceTypeBalance:
		return "balance"
	case database.InvoiceTypeP2P:
		return "p2p"
	default:
		provider, _ := integrationProviderForInvoiceType(invoiceType)
		return provider
	}
}
