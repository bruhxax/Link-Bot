package payment

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"link-bot/internal/database"
	"link-bot/internal/integrations"
)

// Provider-managed subscriptions send a distinct signed payment event for
// every charge. The source purchase holds the first payment; later event IDs
// become unique external payment IDs on fresh purchases.
func (s PaymentService) processManagedRecurringWebhook(ctx context.Context, provider string, event integrations.WebhookPayment) (string, error) {
	if event.SubscriptionID == "" {
		return "", errors.New("missing recurring payment identity")
	}
	if event.SubscriptionStopped {
		if err := s.customerRepository.MarkRecurringSubscriptionStopped(ctx, provider, event.SubscriptionID); err != nil {
			return "", err
		}
		subscription, err := s.customerRepository.FindRecurringSubscription(ctx, provider, event.SubscriptionID)
		if err != nil || subscription == nil {
			return "OK", err
		}
		method, err := s.customerRepository.FindRecurringMethod(ctx, subscription.CustomerID)
		if err != nil {
			return "", err
		}
		if method != nil && method.Provider == provider && s.integrationSettings != nil {
			if reference, err := s.integrationSettings.DecryptRecurringReference(method.Reference); err == nil && reference == event.SubscriptionID {
				return "OK", s.customerRepository.SetRecurringEnabled(ctx, subscription.CustomerID, false)
			}
		}
		return "OK", nil
	}
	if !event.Paid {
		return "OK", nil
	}
	if event.ExternalID == "" {
		return "", errors.New("missing recurring charge ID")
	}
	if event.Amount <= 0 || !strings.EqualFold(event.Currency, "RUB") {
		return "", errors.New("invalid recurring payment amount or currency")
	}
	invoiceType, ok := invoiceTypeForIntegrationProvider(provider)
	if !ok {
		return "", errors.New("unknown recurring provider")
	}
	subscription, err := s.customerRepository.FindRecurringSubscription(ctx, provider, event.SubscriptionID)
	if err != nil {
		return "", err
	}
	if subscription == nil && provider == integrations.ProviderWata && event.PurchaseID > 0 {
		source, err := s.purchaseRepository.FindById(ctx, event.PurchaseID)
		if err != nil || source == nil || source.InvoiceType != invoiceType {
			return "", errors.New("WATA source purchase not found")
		}
		enrolled, err := s.customerRepository.HasRecurringEnrollment(ctx, source.ID, provider)
		if err != nil || !enrolled {
			return "", errors.New("WATA recurring consent not found")
		}
		subscription = &database.RecurringSubscription{Provider: provider, ExternalID: event.SubscriptionID, CustomerID: source.CustomerID, SourcePurchaseID: source.ID, PlanID: source.PlanID, Months: source.Month, Amount: source.Amount, Active: true}
		if err := s.customerRepository.SaveRecurringSubscription(ctx, *subscription); err != nil {
			return "", err
		}
		stored, err := s.customerRepository.FindRecurringSubscription(ctx, provider, event.SubscriptionID)
		if err != nil || stored == nil || stored.CustomerID != source.CustomerID || stored.SourcePurchaseID != source.ID {
			return "", errors.New("WATA recurring subscription ownership mismatch")
		}
		subscription = stored
	}
	if subscription == nil {
		return "", errors.New("recurring subscription not found")
	}
	if math.Abs(subscription.Amount-event.Amount) > 0.01 {
		return "", errors.New("recurring payment amount mismatch")
	}
	release, err := s.purchaseRepository.LockAutoPaymentCustomer(ctx, subscription.CustomerID)
	if err != nil {
		return "", err
	}
	defer release()
	source, err := s.purchaseRepository.FindById(ctx, subscription.SourcePurchaseID)
	if err != nil || source == nil || source.CustomerID != subscription.CustomerID || source.InvoiceType != invoiceType {
		return "", errors.New("recurring source purchase mismatch")
	}
	purchase, err := s.purchaseRepository.FindByExternalPaymentID(ctx, invoiceType, event.ExternalID)
	if err != nil {
		return "", err
	}
	if purchase == nil {
		if source.Status != database.PurchaseStatusPaid && source.PaidAt == nil {
			if err := s.purchaseRepository.UpdateFields(ctx, source.ID, map[string]interface{}{"external_payment_id": event.ExternalID, "status": database.PurchaseStatusPending}); err != nil {
				return "", err
			}
			purchase = source
		} else {
			paymentID := event.ExternalID
			newPurchase := &database.Purchase{
				InvoiceType: invoiceType, Status: database.PurchaseStatusPending,
				Amount: subscription.Amount, Currency: "RUB", CustomerID: subscription.CustomerID,
				SubscriptionID: source.SubscriptionID, Month: subscription.Months, PlanID: subscription.PlanID,
				AgreementAccepted: true, IsAutoPayment: true, ParentPurchaseID: &source.ID,
				ExternalPaymentID: &paymentID,
			}
			id, err := s.purchaseRepository.Create(ctx, newPurchase)
			if err != nil {
				// The unique (invoice_type, external_payment_id) index handles a
				// concurrent retry that reached another bot instance first.
				purchase, err = s.purchaseRepository.FindByExternalPaymentID(ctx, invoiceType, event.ExternalID)
				if err != nil || purchase == nil {
					return "", fmt.Errorf("create recurring renewal: %w", err)
				}
			} else {
				newPurchase.ID = id
				purchase = newPurchase
			}
		}
	}
	if purchase.CustomerID != subscription.CustomerID || math.Abs(purchase.Amount-event.Amount) > 0.01 {
		return "", errors.New("recurring payment purchase mismatch")
	}
	if err := s.ProcessPurchaseById(ctx, purchase.ID); err != nil {
		return "", err
	}
	if purchase.ID == source.ID {
		if err := s.finishManagedRecurringEnrollment(ctx, source, subscription); err != nil {
			return "", err
		}
	}
	return "OK", nil
}

func (s PaymentService) finishManagedRecurringEnrollment(ctx context.Context, source *database.Purchase, subscription *database.RecurringSubscription) error {
	if !subscription.Active {
		return nil
	}
	if s.runtimeSettings == nil || !s.runtimeSettings.FeatureEnabled("recurring_payments") {
		if err := s.integrationGateway.StopRecurringSubscription(ctx, subscription.Provider, subscription.ExternalID); err != nil {
			return err
		}
		return s.customerRepository.MarkRecurringSubscriptionStopped(ctx, subscription.Provider, subscription.ExternalID)
	}
	if s.integrationSettings == nil {
		return errors.New("recurring reference encryption is unavailable")
	}
	reference, err := s.integrationSettings.EncryptRecurringReference(subscription.ExternalID)
	if err != nil {
		return err
	}
	title := "WATA"
	if subscription.Provider == integrations.ProviderLava {
		title = "LAVA"
	} else if subscription.Provider == integrations.ProviderPlatega {
		title = "Platega · СБП"
	} else if subscription.Provider == integrations.ProviderRollyPay {
		title = "RollyPay · СБП"
	} else if subscription.Provider == integrations.ProviderCisPay {
		title = "cisPay · СБП"
	}
	return s.saveRecurringMethod(ctx, database.RecurringMethod{CustomerID: source.CustomerID, Provider: subscription.Provider, Reference: reference, Title: title, PlanID: subscription.PlanID, SourcePurchaseID: source.ID, Months: subscription.Months})
}

// LAVA announces subscription state changes, while its status endpoint reports
// the time of the latest successful charge. Polling that endpoint supplies a
// stable charge identity even if a webhook is delayed or repeated.
func (s PaymentService) SyncLavaRecurringSubscriptions(ctx context.Context) error {
	if s.integrationGateway == nil || s.customerRepository == nil {
		return nil
	}
	subscriptions, err := s.customerRepository.ClaimDueLavaSubscriptions(ctx, 50)
	if err != nil {
		return err
	}
	var failures []error
	for _, subscription := range subscriptions {
		event, err := s.integrationGateway.LavaRecurringStatus(ctx, subscription.ExternalID, subscription.SourcePurchaseID)
		if err == nil && event.Paid {
			paidEvent := event
			paidEvent.SubscriptionStopped = false
			_, err = s.processManagedRecurringWebhook(ctx, integrations.ProviderLava, paidEvent)
		}
		if err == nil && event.SubscriptionStopped {
			_, err = s.processManagedRecurringWebhook(ctx, integrations.ProviderLava, integrations.WebhookPayment{SubscriptionID: subscription.ExternalID, SubscriptionStopped: true})
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("LAVA subscription %s: %w", subscription.ExternalID, err))
		}
	}
	return errors.Join(failures...)
}
