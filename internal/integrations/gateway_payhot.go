package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Invalid signed events must not be acknowledged as completed payments.
var ErrInvalidPayHotWebhook = errors.New("invalid PayHot webhook")

var payHotSignaturePattern = regexp.MustCompile(`^t=([0-9]{1,12}),v2=([a-f0-9]{64})$`)
var payHotDecimalPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

type payHotPayment struct {
	ID       string `json:"id"`
	Order    string `json:"merchant_order_reference"`
	Amount   string `json:"amount_minor"`
	Captured string `json:"captured_amount_minor"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Object   string `json:"object"`
}

type payHotCheckoutResponse struct {
	Payment  payHotPayment `json:"payment"`
	Checkout struct {
		URL             string `json:"url"`
		ReissueRequired bool   `json:"reissue_required"`
	} `json:"checkout"`
}

func normalizePayHotConfig(cfg map[string]string) error {
	method := strings.ToLower(strings.TrimSpace(firstNonEmpty(cfg["paymentMethod"], "sbp")))
	switch method {
	case "sbp", "card", "sberpay", "crypto", "applepay", "googlepay":
		cfg["paymentMethod"] = method
	default:
		return errors.New("для PayHot выберите СБП, карту, SberPay, криптовалюту, Apple Pay или Google Pay")
	}
	if strings.HasPrefix(cfg["apiKey"], "phk_test_") {
		return errors.New("для PayHot нужен рабочий API-ключ; тестовые платежи не оплачивают подписку")
	}
	return nil
}

func payHotMinorAmount(value string) (int64, error) {
	if !payHotDecimalPattern.MatchString(value) {
		return 0, errors.New("invalid PayHot amount")
	}
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount > 1<<53-1 {
		return 0, errors.New("invalid PayHot amount")
	}
	return amount, nil
}

func (g *Gateway) createPayHot(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	if err := normalizePayHotConfig(cfg); err != nil {
		return CreatedPayment{}, err
	}
	if input.PurchaseID <= 0 || input.Currency != "RUB" || input.Amount <= 0 || math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) || input.Amount*100 > 1<<53-1 {
		return CreatedPayment{}, errors.New("invalid PayHot order or RUB amount")
	}
	minor := int64(math.Round(input.Amount * 100))
	if minor <= 0 || math.Abs(input.Amount*100-float64(minor)) > 0.000001 {
		return CreatedPayment{}, errors.New("PayHot amount must have at most two decimal places")
	}
	order := strconv.FormatInt(input.PurchaseID, 10)
	body := map[string]any{"merchant_order_reference": order, "amount_minor": strconv.FormatInt(minor, 10), "currency": "RUB", "payment_method": cfg["paymentMethod"]}
	if input.CustomerID > 0 {
		customer := map[string]string{"id": strconv.FormatInt(input.CustomerID, 10)}
		if input.Email != "" {
			customer["email"] = input.Email
		}
		body["customer"] = customer
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return CreatedPayment{}, err
	}
	base := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://app.pay.hot"), "/")
	headers := map[string]string{"Authorization": "Bearer " + cfg["apiKey"], "Idempotency-Key": "link-bot-payhot-order-" + order}
	var response payHotCheckoutResponse
	if err := g.doJSON(ctx, http.MethodPost, base+"/api/v2/payments", raw, headers, &response); err != nil {
		return CreatedPayment{}, err
	}
	validate := func() bool {
		_, err := uuid.Parse(response.Payment.ID)
		return err == nil && response.Payment.Order == order && response.Payment.Amount == strconv.FormatInt(minor, 10) && response.Payment.Currency == "RUB"
	}
	if !validate() {
		return CreatedPayment{}, errors.New("PayHot returned a mismatched payment")
	}
	if response.Checkout.ReissueRequired {
		// A replay returns no access token. Reissue access to the same payment,
		// rather than creating another order after a lost response.
		id := response.Payment.ID
		response = payHotCheckoutResponse{}
		if err := g.doJSON(ctx, http.MethodPost, base+"/api/v2/payments/"+url.PathEscape(id)+"/checkout-sessions", nil, map[string]string{"Authorization": headers["Authorization"]}, &response); err != nil {
			return CreatedPayment{}, err
		}
		if response.Payment.ID != id || !validate() {
			return CreatedPayment{}, errors.New("PayHot returned a mismatched checkout session")
		}
	}
	u, err := url.Parse(response.Checkout.URL)
	if err != nil || !validCheckoutURL(response.Checkout.URL) || u.Host != "app.pay.hot" || !strings.HasPrefix(u.Path, "/pay/") || !strings.HasPrefix(u.Fragment, "token=") || len(u.Fragment) <= len("token=") {
		return CreatedPayment{}, errors.New("PayHot did not return a valid payment link")
	}
	return CreatedPayment{ExternalID: response.Payment.ID, URL: response.Checkout.URL}, nil
}

func parsePayHotWebhook(cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	event, err := verifyPayHotWebhook(cfg, headers, raw, time.Now())
	if err != nil {
		return WebhookPayment{}, fmt.Errorf("%w: %v", ErrInvalidPayHotWebhook, err)
	}
	return event, nil
}

func verifyPayHotWebhook(cfg map[string]string, headers http.Header, raw []byte, now time.Time) (WebhookPayment, error) {
	parts := payHotSignaturePattern.FindStringSubmatch(headers.Get("PayHot-Signature"))
	if len(parts) != 3 || cfg["webhookSecret"] == "" {
		return WebhookPayment{}, errors.New("missing or malformed signature")
	}
	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || timestamp < now.Unix()-300 || timestamp > now.Unix()+300 {
		return WebhookPayment{}, errors.New("expired signature")
	}
	signed := append([]byte(parts[1]+"."), raw...)
	if !hmac.Equal([]byte(parts[2]), []byte(hmacHex(sha256.New, []byte(cfg["webhookSecret"]), signed))) {
		return WebhookPayment{}, errors.New("invalid signature")
	}
	var payload struct {
		Version     string        `json:"api_version"`
		EventID     string        `json:"event_id"`
		EventType   string        `json:"event_type"`
		Environment string        `json:"environment"`
		Data        payHotPayment `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	if payload.Version != "v2" || payload.EventID == "" || (headers.Get("PayHot-Event-Id") != "" && headers.Get("PayHot-Event-Id") != payload.EventID) {
		return WebhookPayment{}, errors.New("invalid event envelope")
	}
	// Test payments and other transaction types never fulfil real orders.
	if payload.Environment == "sandbox" || !strings.HasPrefix(payload.EventType, "payment.") {
		return WebhookPayment{}, errors.New("unsupported payment event")
	}
	data := payload.Data
	if _, err := uuid.Parse(data.ID); err != nil || data.Object != "payment" || data.Currency != "RUB" {
		return WebhookPayment{}, errors.New("invalid payment identity or currency")
	}
	purchaseID, err := strconv.ParseInt(data.Order, 10, 64)
	if err != nil || purchaseID <= 0 || strconv.FormatInt(purchaseID, 10) != data.Order {
		return WebhookPayment{}, errors.New("invalid order reference")
	}
	minor, err := payHotMinorAmount(data.Amount)
	if err != nil {
		return WebhookPayment{}, err
	}
	paid := payload.EventType == "payment.succeeded" && data.Status == "succeeded"
	if paid {
		captured, err := payHotMinorAmount(data.Captured)
		if err != nil || captured != minor {
			return WebhookPayment{}, errors.New("payment was not fully captured")
		}
	}
	// Failed/cancelled events do not invalidate an order: PayHot may later
	// confirm success. Refunds and partial captures never grant a subscription.
	return WebhookPayment{PurchaseID: purchaseID, ExternalID: data.ID, Amount: float64(minor) / 100, Currency: data.Currency, Paid: paid}, nil
}
