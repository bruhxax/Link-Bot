package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func (g *Gateway) createDatagio(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	if input.Amount <= 0 || math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) {
		return CreatedPayment{}, errors.New("invalid Datagio amount")
	}
	if input.Currency != "RUB" {
		return CreatedPayment{}, errors.New("Datagio supports RUB only")
	}
	shopID, err := strconv.ParseInt(cfg["shopId"], 10, 64)
	if err != nil || shopID <= 0 {
		return CreatedPayment{}, errors.New("invalid Datagio shop ID")
	}
	raw, _ := json.Marshal(map[string]any{
		"shop_id": shopID, "amount": formatAmount(input.Amount), "order_id": strconv.FormatInt(input.PurchaseID, 10),
		"description": input.Description, "expires_in_minutes": 60,
	})
	var response struct {
		ID          string `json:"id"`
		CheckoutURL string `json:"checkout_url"`
		PayURL      string `json:"pay_url"`
		PaymentURL  string `json:"paymentUrl"`
	}
	endpoint := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://api.datagio.finance"), "/") + "/merchant/v1/payment-links"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"X-API-Key": cfg["apiKey"], "X-API-Secret": cfg["apiSecret"]}, &response); err != nil {
		return CreatedPayment{}, err
	}
	link := firstNonEmpty(response.CheckoutURL, response.PayURL, response.PaymentURL)
	if response.ID == "" || !validCheckoutURL(link) {
		return CreatedPayment{}, errors.New("Datagio did not return a payment link")
	}
	return CreatedPayment{ExternalID: response.ID, URL: link}, nil
}

func parseDatagioWebhook(cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	signature := strings.ToLower(firstNonEmpty(headers.Get("X-Webhook-Signature"), headers.Get("Datagio-Signature")))
	expected := hmacHex(sha256.New, []byte(cfg["webhookSecret"]), raw)
	if cfg["webhookSecret"] == "" || signature == "" || !hmac.Equal([]byte(signature), []byte(expected)) {
		return WebhookPayment{}, errors.New("invalid Datagio webhook signature")
	}
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			ID            string      `json:"id"`
			PaymentLinkID string      `json:"payment_link_id"`
			ShopID        json.Number `json:"shop_id"`
			OrderID       string      `json:"order_id"`
			Amount        json.Number `json:"amount"`
			Status        string      `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	data := payload.Data
	amount, err := data.Amount.Float64()
	if err != nil || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || data.ShopID.String() != cfg["shopId"] {
		return WebhookPayment{}, errors.New("invalid Datagio payment")
	}
	externalID := firstNonEmpty(data.PaymentLinkID, data.ID)
	if externalID == "" || (data.ID != "" && data.PaymentLinkID != "" && data.ID != data.PaymentLinkID) {
		return WebhookPayment{}, errors.New("invalid Datagio payment ID")
	}
	var purchaseID int64
	if data.OrderID != "" {
		purchaseID, err = strconv.ParseInt(data.OrderID, 10, 64)
		if err != nil || purchaseID <= 0 {
			return WebhookPayment{}, errors.New("invalid Datagio order ID")
		}
	}
	return WebhookPayment{PurchaseID: purchaseID, ExternalID: externalID, Amount: amount, Currency: "RUB", Paid: payload.Event == "payment.succeeded" && data.Status == "paid"}, nil
}

var kassaAINonce atomic.Int64

func nextKassaAINonce() int64 {
	for {
		previous := kassaAINonce.Load()
		next := time.Now().UnixNano()
		if next <= previous {
			next = previous + 1
		}
		if kassaAINonce.CompareAndSwap(previous, next) {
			return next
		}
	}
}

func (g *Gateway) createKassaAI(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	if input.Amount <= 0 || math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) {
		return CreatedPayment{}, errors.New("invalid Kassa AI amount")
	}
	if input.Currency != "RUB" {
		return CreatedPayment{}, errors.New("Kassa AI supports RUB only")
	}
	shopID, err := strconv.ParseInt(cfg["shopId"], 10, 64)
	if err != nil || shopID <= 0 {
		return CreatedPayment{}, errors.New("invalid Kassa AI shop ID")
	}
	method, err := strconv.Atoi(firstNonEmpty(cfg["paymentSystemId"], "44"))
	if err != nil || (method != 44 && method != 36 && method != 43) {
		return CreatedPayment{}, errors.New("invalid Kassa AI payment method")
	}
	ip := firstNonEmpty(input.ClientIP, cfg["clientIP"])
	if net.ParseIP(ip) == nil {
		return CreatedPayment{}, errors.New("Kassa AI requires a valid payer IP; configure clientIP for bot payments")
	}
	amount := strconv.FormatFloat(math.Round(input.Amount*100)/100, 'f', -1, 64)
	params := map[string]any{
		"shopId": shopID, "nonce": nextKassaAINonce(), "paymentId": strconv.FormatInt(input.PurchaseID, 10),
		"i": method, "email": firstNonEmpty(input.Email, cfg["email"], fmt.Sprintf("%d@telegram.org", input.CustomerID)),
		"ip": ip, "amount": json.Number(amount), "currency": input.Currency,
		"notification_url": g.settings.WebhookURL(ProviderKassaAI),
	}
	if input.ReturnURL != "" {
		params["success_url"] = input.ReturnURL
		params["failure_url"] = input.ReturnURL
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, fmt.Sprint(params[key]))
	}
	params["signature"] = hmacHex(sha256.New, []byte(cfg["apiKey"]), []byte(strings.Join(values, "|")))
	raw, _ := json.Marshal(params)
	var response struct {
		Type     string      `json:"type"`
		Location string      `json:"location"`
		OrderID  json.Number `json:"orderId"`
	}
	endpoint := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://api.fk.life/v1"), "/") + "/orders/create"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, nil, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.Type != "success" || response.OrderID.String() == "" || !validCheckoutURL(response.Location) {
		return CreatedPayment{}, errors.New("Kassa AI did not return a payment link")
	}
	return CreatedPayment{ExternalID: response.OrderID.String(), URL: response.Location}, nil
}

func parseKassaAIWebhook(cfg map[string]string, form url.Values) (WebhookPayment, error) {
	merchant, amount, orderID := form.Get("MERCHANT_ID"), form.Get("AMOUNT"), form.Get("MERCHANT_ORDER_ID")
	expected := fmt.Sprintf("%x", md5.Sum([]byte(strings.Join([]string{merchant, amount, cfg["secretWord2"], orderID}, ":"))))
	if cfg["secretWord2"] == "" || merchant != cfg["shopId"] || !hmac.Equal([]byte(expected), []byte(strings.ToLower(form.Get("SIGN")))) {
		return WebhookPayment{}, errors.New("invalid Kassa AI webhook signature")
	}
	purchaseID, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil || purchaseID <= 0 || form.Get("intid") == "" {
		return WebhookPayment{}, errors.New("invalid Kassa AI order")
	}
	value, err := strconv.ParseFloat(amount, 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return WebhookPayment{}, errors.New("invalid Kassa AI amount")
	}
	return WebhookPayment{PurchaseID: purchaseID, ExternalID: form.Get("intid"), Amount: value, Currency: "RUB", Paid: true}, nil
}

func validCheckoutURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}
