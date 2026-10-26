package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"link-bot/internal/database"
)

var ErrInvalidLaveshkaWebhook = errors.New("invalid Laveshka webhook")
var laveKeyPattern = regexp.MustCompile(`^LAVE-([1-9][0-9]*)-[A-Za-z0-9_-]+$`)
var laveRatePattern = regexp.MustCompile(`^[0-9]{1,7}(\.[0-9]{1,6})?$`)
var laveMoneyPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,7})(\.[0-9]{1,2})?$`)
var laveInvoicePattern = regexp.MustCompile(`^inv_[A-Za-z0-9_-]{8,128}$`)
var laveCheckoutPattern = regexp.MustCompile(`^lpay_[A-Za-z0-9_-]{8,128}$`)

type laveshkaQuoteStore interface {
	SaveLaveshkaQuote(context.Context, database.LaveshkaQuote) error
	FindLaveshkaQuote(context.Context, string) (*database.LaveshkaQuote, error)
}

// Only fixed local codes reach logs or the UI; upstream errors may contain secrets.
type LaveshkaPaymentError struct {
	StatusCode int
	Code       string
}

func (e *LaveshkaPaymentError) Error() string {
	return fmt.Sprintf("Laveshka: %s (HTTP %d)", e.Code, e.StatusCode)
}
func (e *LaveshkaPaymentError) PublicMessage() string {
	if e.Code == "amount_out_of_range" {
		return "Лавешка принимает платежи от 0,10 до 10 000 USD. Выберите другой тариф или способ оплаты."
	}
	return "Не удалось создать платёж Лавешки. Попробуйте позже или выберите другой способ оплаты."
}
func (e *LaveshkaPaymentError) AdminMessage() string {
	return e.PublicMessage() + " Проверьте API-ключ, секрет вебхука и курс ₽/USD в интеграции. " + e.Error()
}

func laveMerchant(key string) (int64, error) {
	parts := laveKeyPattern.FindStringSubmatch(key)
	if len(parts) != 2 {
		return 0, errors.New("Лавешка: API-ключ должен иметь вид LAVE-ID-секрет")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("Лавешка: некорректный ID магазина в API-ключе")
	}
	return id, nil
}

func normalizeLaveshkaConfig(cfg map[string]string) error {
	if cfg["apiKey"] != "" {
		if _, err := laveMerchant(cfg["apiKey"]); err != nil {
			return err
		}
	}
	if rate := cfg["rubPerUsd"]; rate != "" {
		rate = strings.ReplaceAll(strings.TrimSpace(rate), ",", ".")
		r, ok := new(big.Rat).SetString(rate)
		if !laveRatePattern.MatchString(rate) || !ok || r.Sign() <= 0 || r.Cmp(big.NewRat(1000000, 1)) > 0 {
			return errors.New("Лавешка: укажите положительный курс ₽/USD, до 6 знаков после запятой")
		}
		cfg["rubPerUsd"] = r.FloatString(6)
	}
	return nil
}

func laveAmountMinor(value string) (int64, bool) {
	if !laveMoneyPattern.MatchString(value) {
		return 0, false
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return 0, false
	}
	r.Mul(r, big.NewRat(100, 1))
	return r.Num().Int64(), r.IsInt() && r.Num().IsInt64()
}

func laveConvert(amount float64, rate string) (rub, usd int64, err error) {
	invalid := &LaveshkaPaymentError{Code: "invalid_amount_or_rate"}
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount*100 > 1<<53-1 {
		return 0, 0, invalid
	}
	rub = int64(math.Round(amount * 100))
	if rub <= 0 || math.Abs(amount*100-float64(rub)) > 0.000001 {
		return 0, 0, invalid
	}
	cfg := map[string]string{"rubPerUsd": rate}
	if normalizeLaveshkaConfig(cfg) != nil || cfg["rubPerUsd"] == "" {
		return 0, 0, invalid
	}
	r, _ := new(big.Rat).SetString(cfg["rubPerUsd"])
	cents := new(big.Rat).Quo(new(big.Rat).SetInt64(rub), r)
	whole, rem := new(big.Int), new(big.Int)
	whole.QuoRem(cents.Num(), cents.Denom(), rem)
	if rem.Sign() > 0 {
		whole.Add(whole, big.NewInt(1))
	}
	if !whole.IsInt64() || whole.Int64() < 10 || whole.Int64() > 1000000 {
		return 0, 0, &LaveshkaPaymentError{Code: "amount_out_of_range"}
	}
	return rub, whole.Int64(), nil
}

type laveInvoice struct {
	ID         string `json:"id"`
	MerchantID int64  `json:"merchant_id"`
	Status     string `json:"status"`
	Currency   string `json:"currency"`
	FaceAmount string `json:"face_amount"`
	OrderID    string `json:"order_id"`
	PayURL     string `json:"pay_url"`
}

func (g *Gateway) laveJSON(ctx context.Context, cfg map[string]string, method, path string, raw []byte, target any) error {
	var response struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	err := g.doJSON(ctx, method, "https://laveshka.io/merchant/v1"+path, raw, map[string]string{"X-Api-Key": cfg["apiKey"]}, &response)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var api *PaymentAPIError
		if errors.As(err, &api) {
			return &LaveshkaPaymentError{StatusCode: api.StatusCode, Code: "payment_api_error"}
		}
		return &LaveshkaPaymentError{Code: "payment_api_unavailable"}
	}
	if !response.OK || json.Unmarshal(response.Data, target) != nil {
		return &LaveshkaPaymentError{Code: "invalid_api_response"}
	}
	return nil
}

func validLaveCheckout(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "t.me" || u.User != nil || u.Path != "/laveshka" || u.Fragment != "" {
		return false
	}
	q, err := url.ParseQuery(u.RawQuery)
	return err == nil && len(q) == 1 && len(q["startapp"]) == 1 && laveCheckoutPattern.MatchString(q.Get("startapp"))
}

func (g *Gateway) createLaveshka(ctx context.Context, cfg map[string]string, in CreatePaymentRequest) (CreatedPayment, error) {
	merchant, err := laveMerchant(cfg["apiKey"])
	if err != nil || cfg["webhookSecret"] == "" || g.laveshkaQuotes == nil || in.PurchaseID <= 0 || in.Currency != "RUB" {
		return CreatedPayment{}, &LaveshkaPaymentError{Code: "invalid_configuration"}
	}
	rub, usd, err := laveConvert(in.Amount, cfg["rubPerUsd"])
	if err != nil {
		return CreatedPayment{}, err
	}
	order := strconv.FormatInt(in.PurchaseID, 10)
	desc := []rune(firstNonEmpty(in.Description, "Заказ #"+order))
	if len(desc) > 256 {
		desc = desc[:256]
	}
	payload := map[string]any{"amount": fmt.Sprintf("%d.%02d", usd/100, usd%100), "currency": "USD", "order_id": order, "description": string(desc), "ttl": 3600}
	if validCheckoutURL(in.ReturnURL) && len(in.ReturnURL) <= 512 {
		payload["return_url"] = in.ReturnURL
	}
	raw, _ := json.Marshal(payload)
	var invoice laveInvoice
	if err = g.laveJSON(ctx, cfg, http.MethodPost, "/invoices", raw, &invoice); err != nil {
		return CreatedPayment{}, err
	}
	face, ok := laveAmountMinor(invoice.FaceAmount)
	if !ok || face != usd || invoice.MerchantID != merchant || invoice.OrderID != order || invoice.Currency != "USD" || invoice.Status != "active" || !laveInvoicePattern.MatchString(invoice.ID) || !validLaveCheckout(invoice.PayURL) {
		return CreatedPayment{}, &LaveshkaPaymentError{Code: "payment_response_mismatch"}
	}
	if err = g.laveshkaQuotes.SaveLaveshkaQuote(ctx, database.LaveshkaQuote{PurchaseID: in.PurchaseID, InvoiceID: invoice.ID, MerchantID: merchant, RubMinor: rub, USDMinor: usd}); err != nil {
		return CreatedPayment{}, &LaveshkaPaymentError{Code: "quote_storage_failed"}
	}
	return CreatedPayment{ExternalID: invoice.ID, URL: invoice.PayURL}, nil
}

func (g *Gateway) parseLaveshkaWebhook(ctx context.Context, cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	invalid := ErrInvalidLaveshkaWebhook
	timestamp := headers.Get("X-LavePay-Timestamp")
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	now := time.Now().Unix()
	if err != nil || ts < now-300 || ts > now+60 || cfg["webhookSecret"] == "" {
		return WebhookPayment{}, invalid
	}
	signature := headers.Get("X-LavePay-Signature")
	if !strings.HasPrefix(signature, "v1=") {
		return WebhookPayment{}, invalid
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(signature, "v1="))
	mac := hmac.New(sha256.New, []byte(cfg["webhookSecret"]))
	mac.Write([]byte(timestamp + "."))
	mac.Write(raw)
	if err != nil || !hmac.Equal(digest, mac.Sum(nil)) {
		return WebhookPayment{}, invalid
	}
	var event struct {
		ID         string          `json:"id"`
		Type       string          `json:"type"`
		MerchantID int64           `json:"merchant_id"`
		Data       json.RawMessage `json:"data"`
	}
	merchant, err := laveMerchant(cfg["apiKey"])
	if err != nil || json.Unmarshal(raw, &event) != nil || !strings.HasPrefix(event.ID, "evt_") || event.MerchantID != merchant {
		return WebhookPayment{}, invalid
	}
	if id := headers.Get("X-LavePay-Event-Id"); id != "" && id != event.ID {
		return WebhookPayment{}, invalid
	}
	if kind := headers.Get("X-LavePay-Event"); kind != "" && kind != event.Type {
		return WebhookPayment{}, invalid
	}
	expected := map[string]string{"invoice.paid": "paid", "invoice.expired": "expired", "invoice.cancelled": "cancelled"}[event.Type]
	if expected == "" {
		return WebhookPayment{Ignored: true}, nil
	}
	var invoice laveInvoice
	if json.Unmarshal(event.Data, &invoice) != nil || invoice.Status != expected || invoice.MerchantID != merchant || !laveInvoicePattern.MatchString(invoice.ID) {
		return WebhookPayment{}, invalid
	}
	amount, ok := laveAmountMinor(invoice.FaceAmount)
	if !ok || amount < 10 || amount > 1000000 || invoice.Currency != "USD" {
		return WebhookPayment{}, invalid
	}
	if g.laveshkaQuotes == nil {
		return WebhookPayment{}, &LaveshkaPaymentError{Code: "quote_storage_unavailable"}
	}
	quote, err := g.laveshkaQuotes.FindLaveshkaQuote(ctx, invoice.ID)
	if err != nil {
		return WebhookPayment{}, &LaveshkaPaymentError{Code: "quote_storage_unavailable"}
	}
	if quote == nil {
		return WebhookPayment{}, &LaveshkaPaymentError{Code: "quote_not_ready"}
	}
	if quote.InvoiceID != invoice.ID || quote.MerchantID != merchant || quote.USDMinor != amount || quote.RubMinor <= 0 || quote.PurchaseID <= 0 || invoice.OrderID != strconv.FormatInt(quote.PurchaseID, 10) {
		return WebhookPayment{}, invalid
	}
	var verified laveInvoice
	if err = g.laveJSON(ctx, cfg, http.MethodGet, "/invoices/"+invoice.ID, nil, &verified); err != nil {
		return WebhookPayment{}, err
	}
	verifiedAmount, valid := laveAmountMinor(verified.FaceAmount)
	if !valid || verifiedAmount != amount || verified.ID != invoice.ID || verified.MerchantID != merchant || verified.Currency != invoice.Currency || verified.OrderID != invoice.OrderID {
		return WebhookPayment{}, invalid
	}
	// A late callback must never grant a refunded invoice or cancel a paid one.
	if verified.Status == "refunded" || (expected != "paid" && verified.Status == "paid") {
		return WebhookPayment{Ignored: true}, nil
	}
	if verified.Status != expected {
		return WebhookPayment{}, &LaveshkaPaymentError{Code: "invoice_status_not_ready"}
	}
	return WebhookPayment{PurchaseID: quote.PurchaseID, ExternalID: quote.InvoiceID, Amount: float64(quote.RubMinor) / 100, Currency: "RUB", Paid: expected == "paid", Cancelled: expected != "paid"}, nil
}
