package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

func lavaAPIBase(cfg map[string]string) string {
	return strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://api.lava.ru"), "/") + "/business/recurrent/"
}

func (g *Gateway) lavaRecurringRequest(ctx context.Context, cfg map[string]string, path string, body any, target any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return g.doJSON(ctx, http.MethodPost, lavaAPIBase(cfg)+path, raw, map[string]string{"Signature": hmacHex(sha256.New, []byte(cfg["secretKey"]), raw)}, target)
}

func (g *Gateway) createLavaSubscription(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	period := map[int]string{1: "one_month", 3: "three_months", 6: "six_months", 12: "year"}[input.RecurringMonths]
	if period == "" || strings.TrimSpace(input.Email) == "" || input.PurchaseID <= 0 || input.Amount <= 0 {
		return CreatedPayment{}, errors.New("LAVA recurring payments require a supported period and payer email")
	}
	var products struct {
		StatusCheck bool `json:"status_check"`
		Data        []struct {
			ID         string  `json:"id"`
			Price      float64 `json:"price"`
			Period     string  `json:"period"`
			PeriodDays int     `json:"periodDays"`
			FreeDays   int     `json:"freeDays"`
			ShopID     string  `json:"shopId"`
			IsActive   bool    `json:"isActive"`
		} `json:"data"`
	}
	if err := g.lavaRecurringRequest(ctx, cfg, "product/list", map[string]string{"shopId": cfg["shopId"]}, &products); err != nil {
		return CreatedPayment{}, err
	}
	if !products.StatusCheck {
		return CreatedPayment{}, errors.New("LAVA recurring product list failed")
	}
	productID := ""
	for _, product := range products.Data {
		if product.IsActive && product.ShopID == cfg["shopId"] && product.Period == period && product.FreeDays == 0 && math.Abs(product.Price-input.Amount) < 0.01 {
			productID = product.ID
			break
		}
	}
	if productID == "" {
		return CreatedPayment{}, errors.New("LAVA has no recurring product matching this price and period")
	}
	consumerID := "linkbot-" + strconv.FormatInt(input.PurchaseID, 10)
	var consumer struct {
		StatusCheck bool `json:"status_check"`
		Data        struct {
			ConsumerID string `json:"consumerId"`
		} `json:"data"`
	}
	if err := g.lavaRecurringRequest(ctx, cfg, "consumer/create", map[string]string{"email": input.Email, "consumerId": consumerID, "shopId": cfg["shopId"]}, &consumer); err != nil {
		return CreatedPayment{}, err
	}
	if !consumer.StatusCheck || consumer.Data.ConsumerID != consumerID {
		return CreatedPayment{}, errors.New("LAVA did not confirm recurring customer")
	}
	var created struct {
		StatusCheck bool `json:"status_check"`
		Data        struct {
			SubscriptionID string  `json:"subscriptionId"`
			URL            string  `json:"url"`
			Amount         float64 `json:"amount"`
		} `json:"data"`
	}
	if err := g.lavaRecurringRequest(ctx, cfg, "subscription/subscribe", map[string]string{"productId": productID, "consumerId": consumerID, "orderId": strconv.FormatInt(input.PurchaseID, 10), "shopId": cfg["shopId"]}, &created); err != nil {
		return CreatedPayment{}, err
	}
	if !created.StatusCheck || created.Data.SubscriptionID == "" || !validCheckoutURL(created.Data.URL) {
		return CreatedPayment{}, errors.New("LAVA did not return a recurring checkout URL")
	}
	if math.Abs(created.Data.Amount-input.Amount) > 0.01 {
		if err := g.stopLavaSubscription(ctx, cfg, created.Data.SubscriptionID); err != nil {
			return CreatedPayment{}, fmt.Errorf("LAVA recurring amount changed and cancellation failed: %w", err)
		}
		return CreatedPayment{}, errors.New("LAVA recurring amount does not match checkout")
	}
	return CreatedPayment{ExternalID: created.Data.SubscriptionID, URL: created.Data.URL, RecurringReference: created.Data.SubscriptionID}, nil
}

func (g *Gateway) stopLavaSubscription(ctx context.Context, cfg map[string]string, subscriptionID string) error {
	var response struct {
		StatusCheck bool `json:"status_check"`
		Data        struct {
			Unsubscribed bool `json:"unsubscribed"`
		} `json:"data"`
	}
	if err := g.lavaRecurringRequest(ctx, cfg, "subscription/unsubscribe", map[string]string{"shopId": cfg["shopId"], "subscriptionId": subscriptionID}, &response); err != nil {
		return err
	}
	if !response.StatusCheck || !response.Data.Unsubscribed {
		return errors.New("LAVA did not confirm subscription cancellation")
	}
	return nil
}

func (g *Gateway) LavaRecurringStatus(ctx context.Context, subscriptionID string, sourcePurchaseID int64) (WebhookPayment, error) {
	cfg, ok := g.settings.StoredConfig(ProviderLava)
	if !ok {
		return WebhookPayment{}, errors.New("LAVA recurring integration is unavailable")
	}
	var response struct {
		StatusCheck bool `json:"status_check"`
		Data        struct {
			SubscriptionID string  `json:"subscriptionId"`
			OrderID        string  `json:"orderId"`
			ConsumerID     string  `json:"consumerId"`
			ShopID         string  `json:"shopId"`
			Status         string  `json:"status"`
			Amount         float64 `json:"amount"`
			LastPayTime    string  `json:"last_pay_time"`
		} `json:"data"`
	}
	if err := g.lavaRecurringRequest(ctx, cfg, "subscription/status", map[string]string{"shopId": cfg["shopId"], "subscriptionId": subscriptionID}, &response); err != nil {
		return WebhookPayment{}, err
	}
	data := response.Data
	if !response.StatusCheck || data.SubscriptionID != subscriptionID || data.OrderID != strconv.FormatInt(sourcePurchaseID, 10) || data.ConsumerID != "linkbot-"+strconv.FormatInt(sourcePurchaseID, 10) || data.ShopID != cfg["shopId"] || data.Amount <= 0 {
		return WebhookPayment{}, errors.New("LAVA recurring status identity mismatch")
	}
	status := strings.ToLower(strings.TrimSpace(data.Status))
	event := WebhookPayment{PurchaseID: sourcePurchaseID, SubscriptionID: subscriptionID, Amount: data.Amount, Currency: "RUB", SubscriptionStopped: status == "deactivated"}
	if data.LastPayTime == "" {
		return event, nil
	}
	// The provider's last successful charge time is the stable identity for
	// this billing cycle. Repeated polls map to the same external payment ID.
	event.ExternalID = subscriptionID + ":" + data.LastPayTime
	event.Paid = true
	return event, nil
}
