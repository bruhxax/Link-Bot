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
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/paymentchannel"
)

var ErrInvalidSmerchWebhook = errors.New("invalid Smerch webhook")
var smerchTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// Keep API response bodies (which can contain keys or payment details) out of
// customer errors, logs and diagnostic notifications.
type SmerchPaymentError struct {
	StatusCode int
	Code       string
}

func (e *SmerchPaymentError) Error() string {
	return fmt.Sprintf("Smerch: %s (HTTP %d)", e.Code, e.StatusCode)
}

func (e *SmerchPaymentError) PublicMessage() string {
	return "Не удалось создать платёж Smerch. Попробуйте позже или выберите другой способ оплаты."
}

func (e *SmerchPaymentError) AdminMessage() string {
	message := "Smerch: проверьте настройки интеграции и доступность выбранного метода в кассе."
	if e.StatusCode == http.StatusUnauthorized {
		message = "Smerch: проверьте рабочий API-ключ, IP сервера в списке разрешённых и синхронизацию времени сервера."
	}
	if e.Code == "missing_payment_url" {
		message = "Smerch не вернул ссылку оплаты. Подключите в кассе метод с платёжной страницей."
	}
	return message + " " + e.Error()
}

func normalizeSmerchConfig(cfg map[string]string) error {
	methods, err := smerchPaymentMethods(cfg)
	if err != nil {
		return err
	}
	cfg["paymentMethod"] = methods[0]
	if strings.HasPrefix(cfg["apiKey"], "test_") {
		return errors.New("Smerch: нужен рабочий API-ключ live_; тестовые платежи не оплачивают подписку")
	}
	if id := cfg["webhookKeyId"]; id != "" && !smerchTokenPattern.MatchString(id) {
		return errors.New("Smerch: некорректный ID ключа вебхука")
	}
	return nil
}

func smerchPaymentMethods(cfg map[string]string) ([]string, error) {
	var methods []string
	if raw := strings.TrimSpace(cfg["paymentMethods"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &methods); err != nil {
			return nil, errors.New("Smerch: выберите способы оплаты")
		}
	} else if legacy := strings.TrimSpace(cfg["paymentMethod"]); legacy != "" {
		methods = []string{legacy}
	} else {
		methods = []string{"sbp", "crypto"}
	}
	if len(methods) == 0 || len(methods) > 20 {
		return nil, errors.New("Smerch: выберите хотя бы один способ оплаты (не более 20)")
	}
	result := make([]string, 0, len(methods))
	for _, method := range methods {
		method = strings.TrimSpace(method)
		if !paymentchannel.Valid(ProviderSmerch, method) {
			return nil, errors.New("Smerch: некорректный код разового метода оплаты")
		}
		if !slices.Contains(result, method) {
			result = append(result, method)
		}
	}
	return result, nil
}

type smerchTransaction struct {
	Environment   string `json:"environment"`
	UUID          string `json:"uuid"`
	ExternalID    string `json:"external_id"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	PaymentMethod string `json:"payment_method"`
	Status        string `json:"status"`
	PaymentURL    string `json:"payment_url"`
}

func smerchDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (g *Gateway) doSmerchJSON(ctx context.Context, cfg map[string]string, method, path string, raw []byte, target any) error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	requestID := uuid.NewString()
	// These endpoints have no query parameters. The empty third line is part
	// of Smerch's HMAC v2 canonical string; sign exactly the bytes sent.
	canonical := strings.Join([]string{method, path, "", requestID, timestamp, smerchDigest(raw)}, "\n")
	headers := map[string]string{
		"X-Api-Key": cfg["apiKey"], "X-Timestamp": timestamp,
		"X-Smerch-Request-Id": requestID, "X-Signature-Version": "v2",
		"X-Signature": hmacHex(sha256.New, []byte(cfg["apiKey"]), []byte(canonical)),
	}
	endpoint := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://api.smerch.io"), "/") + path
	if err := g.doJSON(ctx, method, endpoint, raw, headers, target); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var apiErr *PaymentAPIError
		if errors.As(err, &apiErr) {
			return &SmerchPaymentError{StatusCode: apiErr.StatusCode, Code: "payment_api_error"}
		}
		return &SmerchPaymentError{Code: "payment_api_unavailable"}
	}
	return nil
}

func validSmerchTransaction(t smerchTransaction) bool {
	if strings.EqualFold(t.Environment, "test") || strings.EqualFold(t.Environment, "sandbox") {
		return false
	}
	if id, err := uuid.Parse(t.UUID); err != nil || id == uuid.Nil {
		return false
	}
	order, err := strconv.ParseInt(t.ExternalID, 10, 64)
	return err == nil && order > 0 && strconv.FormatInt(order, 10) == t.ExternalID &&
		t.Amount > 0 && t.Amount <= 1<<53-1 && t.Currency == "RUB" &&
		paymentchannel.Valid(ProviderSmerch, t.PaymentMethod)
}

func (g *Gateway) createSmerch(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	methods, err := smerchPaymentMethods(cfg)
	if err != nil {
		return CreatedPayment{}, err
	}
	if err := normalizeSmerchConfig(cfg); err != nil {
		return CreatedPayment{}, err
	}
	if len(input.Channels) > 0 {
		// Old single-method settings were a default, not a restriction on
		// per-row methods. Preserve those rows until the new list is saved.
		if cfg["paymentMethods"] != "" && !slices.Contains(methods, input.Channels[0]) {
			return CreatedPayment{}, errors.New("Smerch: этот способ оплаты выключен в интеграции")
		}
		cfg["paymentMethod"] = input.Channels[0]
	} else if len(methods) > 1 {
		return CreatedPayment{}, errors.New("Smerch: выберите способ оплаты")
	}
	if input.PurchaseID <= 0 || input.Currency != "RUB" || input.Amount <= 0 || math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) || input.Amount*100 > 1<<53-1 {
		return CreatedPayment{}, errors.New("invalid Smerch order or RUB amount")
	}
	minor := int64(math.Round(input.Amount * 100))
	if minor <= 0 || math.Abs(input.Amount*100-float64(minor)) > 0.000001 {
		return CreatedPayment{}, errors.New("Smerch amount must have at most two decimal places")
	}
	order := strconv.FormatInt(input.PurchaseID, 10)
	description := []rune(firstNonEmpty(input.Description, "Заказ #"+order))
	if len(description) > 255 {
		description = description[:255]
	}
	payload := map[string]any{"amount": minor, "currency": "RUB", "payment_method": cfg["paymentMethod"], "external_id": order, "description": string(description)}
	metadata := map[string]string{}
	if input.CustomerID > 0 {
		metadata["userId"] = "customer-" + strconv.FormatInt(input.CustomerID, 10)
	}
	if input.Username != "" {
		metadata["userName"] = input.Username
	}
	if ip := net.ParseIP(input.ClientIP); ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified() {
		metadata["clientIp"] = ip.String()
	}
	if len(metadata) > 0 {
		payload["metadata"] = metadata
	}
	if callback := g.settings.WebhookURL(ProviderSmerch); validCheckoutURL(callback) {
		payload["callback_url"] = callback
	}
	if validCheckoutURL(input.ReturnURL) {
		payload["success_url"], payload["fail_url"] = input.ReturnURL, input.ReturnURL
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return CreatedPayment{}, err
	}
	var response smerchTransaction
	if err := g.doSmerchJSON(ctx, cfg, http.MethodPost, "/api/v1/integration/transactions", raw, &response); err != nil {
		return CreatedPayment{}, err
	}
	if !validSmerchTransaction(response) || response.ExternalID != order || response.Amount != minor || response.PaymentMethod != cfg["paymentMethod"] {
		return CreatedPayment{}, &SmerchPaymentError{Code: "payment_response_mismatch"}
	}
	if !validCheckoutURL(response.PaymentURL) {
		// Some H2H-only methods return bank/crypto requisites, not a URL.
		// Do not invent a payment page or redirect to an untrusted scheme.
		return CreatedPayment{}, &SmerchPaymentError{Code: "missing_payment_url"}
	}
	return CreatedPayment{ExternalID: response.UUID, URL: response.PaymentURL}, nil
}

func verifySmerchWebhook(cfg map[string]string, headers http.Header, raw []byte, now time.Time) error {
	id, version := headers.Get("X-Webhook-Id"), headers.Get("X-Webhook-Version")
	if parsed, err := uuid.Parse(id); err != nil || parsed == uuid.Nil || !smerchTokenPattern.MatchString(version) || cfg["webhookSecret"] == "" {
		return errors.New("missing webhook identity or secret")
	}
	timestamp := headers.Get("X-Webhook-Timestamp")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds <= 0 || seconds > now.Unix()+300 {
		return errors.New("invalid webhook timestamp")
	}
	// Retries can arrive many hours later. The authenticated GET below checks
	// current state, and purchase fulfilment deduplicates successful orders.
	signature := headers.Get("X-Webhook-Signature")
	if keyID := cfg["webhookKeyId"]; keyID != "" && headers.Get("X-Webhook-Key-Id") != keyID {
		if headers.Get("X-Webhook-Previous-Key-Id") != keyID {
			return errors.New("unknown webhook key")
		}
		signature = headers.Get("X-Webhook-Previous-Signature")
	}
	canonical := strings.Join([]string{version, id, timestamp, smerchDigest(raw)}, "\n")
	want := hmacHex(sha256.New, []byte(cfg["webhookSecret"]), []byte(canonical))
	if !hmac.Equal([]byte(signature), []byte(want)) {
		return errors.New("invalid webhook signature")
	}
	return nil
}

func (g *Gateway) parseSmerchWebhook(ctx context.Context, cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	invalid := func(err error) (WebhookPayment, error) {
		return WebhookPayment{}, fmt.Errorf("%w: %v", ErrInvalidSmerchWebhook, err)
	}
	if err := normalizeSmerchConfig(cfg); err != nil {
		return invalid(err)
	}
	if cfg["webhookSecret"] != "" {
		if err := verifySmerchWebhook(cfg, headers, raw, time.Now()); err != nil {
			return invalid(err)
		}
	}
	var payload struct {
		Event       string            `json:"event"`
		Transaction smerchTransaction `json:"transaction"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return invalid(errors.New("malformed webhook body"))
	}
	if payload.Event != "transaction.status_updated" {
		return WebhookPayment{Ignored: true}, nil
	}
	t := payload.Transaction
	if strings.EqualFold(t.Environment, "test") || strings.EqualFold(t.Environment, "sandbox") {
		return WebhookPayment{Ignored: true}, nil
	}
	if !validSmerchTransaction(t) {
		return invalid(errors.New("invalid transaction identity, amount or currency"))
	}
	if t.Status != "paid" {
		// failed/expired can later become paid. Refunds and partial payments
		// never grant subscriptions or cancel an already completed purchase.
		return WebhookPayment{Ignored: true}, nil
	}
	var current smerchTransaction
	if err := g.doSmerchJSON(ctx, cfg, http.MethodGet, "/api/v1/integration/transactions/"+url.PathEscape(t.UUID), nil, &current); err != nil {
		return WebhookPayment{}, err // A 5xx response lets Smerch retry safely.
	}
	if !validSmerchTransaction(current) || current.UUID != t.UUID || current.ExternalID != t.ExternalID || current.Amount != t.Amount || current.PaymentMethod != t.PaymentMethod {
		return invalid(errors.New("transaction verification mismatch"))
	}
	if current.Status == "refunded" || current.Status == "chargeback" {
		return WebhookPayment{Ignored: true}, nil
	}
	if current.Status != "paid" {
		// A paid callback can race with a replica still showing pending.
		// Acknowledge only after confirmation, so a retry cannot be lost.
		return WebhookPayment{}, &SmerchPaymentError{Code: "transaction_not_confirmed"}
	}
	order, _ := strconv.ParseInt(t.ExternalID, 10, 64)
	return WebhookPayment{PurchaseID: order, ExternalID: t.UUID, Amount: float64(t.Amount) / 100, Currency: t.Currency, Paid: true}, nil
}
