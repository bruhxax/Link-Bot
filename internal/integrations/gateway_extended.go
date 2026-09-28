package integrations

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func (g *Gateway) createAuraPay(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	raw, _ := json.Marshal(map[string]any{
		"amount": input.Amount, "order_id": strconv.FormatInt(input.PurchaseID, 10), "comment": input.Description,
		"callback_url": g.settings.WebhookURL(ProviderAuraPay), "success_url": input.ReturnURL, "fail_url": input.ReturnURL,
	})
	var response struct {
		ID          string `json:"id"`
		PaymentData struct {
			URL string `json:"url"`
		} `json:"payment_data"`
	}
	endpoint := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://app.aurapay.tech"), "/") + "/invoice/create"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"X-ApiKey": cfg["apiKey"], "X-ShopId": cfg["shopId"]}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.ID == "" || response.PaymentData.URL == "" {
		return CreatedPayment{}, errors.New("AuraPay did not return a payment link")
	}
	return CreatedPayment{ExternalID: response.ID, URL: response.PaymentData.URL}, nil
}

func parseShopInvoiceWebhook(cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return WebhookPayment{}, err
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var canonical strings.Builder
	for _, key := range keys {
		if string(fields[key]) == "null" {
			continue
		}
		value, err := jsonScalarString(fields[key])
		if err != nil {
			return WebhookPayment{}, err
		}
		canonical.WriteString(value)
	}
	signature := strings.ToLower(strings.TrimSpace(headers.Get("X-Signature")))
	expected := hmacHex(sha256.New, []byte(cfg["webhookSecret"]), []byte(canonical.String()))
	if signature == "" || !hmac.Equal([]byte(signature), []byte(expected)) {
		return WebhookPayment{}, errors.New("invalid shop invoice webhook signature")
	}
	var payload struct {
		ID      string `json:"id"`
		OrderID string `json:"order_id"`
		ShopID  string `json:"shop_id"`
		Amount  string `json:"amount"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	id, err := strconv.ParseInt(payload.OrderID, 10, 64)
	if err != nil || id <= 0 || payload.ID == "" || payload.ShopID != cfg["shopId"] {
		return WebhookPayment{}, errors.New("invalid shop invoice payment")
	}
	amount, err := strconv.ParseFloat(payload.Amount, 64)
	if err != nil || amount <= 0 {
		return WebhookPayment{}, errors.New("invalid shop invoice amount")
	}
	return WebhookPayment{PurchaseID: id, ExternalID: payload.ID, Amount: amount, Currency: "RUB", Paid: payload.Status == "PAID", Cancelled: payload.Status == "EXPIRED" || payload.Status == "ERROR"}, nil
}

func (g *Gateway) createParityPay(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	raw, _ := json.Marshal(map[string]any{
		"amount": input.Amount, "order_id": strconv.FormatInt(input.PurchaseID, 10), "comment": input.Description, "expire": 60,
		"callback_url": g.settings.WebhookURL(ProviderParityPay), "success_url": input.ReturnURL, "fail_url": input.ReturnURL,
	})
	var response struct {
		ID   string `json:"id"`
		Link string `json:"link"`
	}
	endpoint := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://api.paritypay.net"), "/") + "/v2/invoice/create"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"X-SecretKey": cfg["secretKey"], "X-ShopId": cfg["shopId"]}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.ID == "" || response.Link == "" {
		return CreatedPayment{}, errors.New("ParityPay did not return a payment link")
	}
	return CreatedPayment{ExternalID: response.ID, URL: response.Link}, nil
}

func (g *Gateway) createAntiloPay(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	if input.Email == "" {
		return CreatedPayment{}, errors.New("AntiloPay: укажите email покупателя")
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg["privateKey"]))
	if err != nil {
		return CreatedPayment{}, errors.New("invalid AntiloPay private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return CreatedPayment{}, errors.New("invalid AntiloPay PKCS8 key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return CreatedPayment{}, errors.New("AntiloPay requires an RSA key")
	}
	raw, _ := json.Marshal(map[string]any{
		"project_identificator": cfg["projectId"], "order_id": strconv.FormatInt(input.PurchaseID, 10), "amount": input.Amount,
		"currency": strings.ToLower(input.Currency), "product_name": "Подписка", "product_type": "services", "description": input.Description,
		"customer": map[string]string{"email": input.Email, "ip": input.ClientIP}, "success_url": input.ReturnURL, "fail_url": input.ReturnURL,
	})
	digest := sha256.Sum256(raw)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return CreatedPayment{}, err
	}
	var response struct {
		Code  int    `json:"code"`
		ID    string `json:"payment_id"`
		URL   string `json:"payment_url"`
		Error string `json:"error"`
	}
	endpoint := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://lk.antilopay.com/api/v1"), "/") + "/payment/create"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"X-Apay-Secret-Id": cfg["secretId"], "X-Apay-Sign-Version": "1", "X-Apay-Sign": base64.StdEncoding.EncodeToString(signature)}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.Code != 0 || response.ID == "" || response.URL == "" {
		return CreatedPayment{}, fmt.Errorf("AntiloPay did not return a payment link: %s", response.Error)
	}
	return CreatedPayment{ExternalID: response.ID, URL: response.URL}, nil
}

func parseAntiloPayWebhook(cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	if headers.Get("X-Apay-Callback-Version") != "1" {
		return WebhookPayment{}, errors.New("unsupported AntiloPay callback version")
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg["publicKey"]))
	if err != nil {
		return WebhookPayment{}, errors.New("invalid AntiloPay public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return WebhookPayment{}, err
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return WebhookPayment{}, errors.New("AntiloPay requires an RSA public key")
	}
	signature, err := base64.StdEncoding.DecodeString(headers.Get("X-Apay-Callback"))
	if err != nil {
		return WebhookPayment{}, err
	}
	digest := sha256.Sum256(raw)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return WebhookPayment{}, errors.New("invalid AntiloPay callback signature")
	}
	var payload struct {
		Type     string  `json:"type"`
		ID       string  `json:"payment_id"`
		OrderID  string  `json:"order_id"`
		Amount   float64 `json:"original_amount"`
		Currency string  `json:"currency"`
		Status   string  `json:"status"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	id, err := strconv.ParseInt(payload.OrderID, 10, 64)
	if err != nil || id <= 0 || payload.Type != "payment" || payload.ID == "" || payload.Amount <= 0 {
		return WebhookPayment{}, errors.New("invalid AntiloPay payment callback")
	}
	return WebhookPayment{PurchaseID: id, ExternalID: payload.ID, Amount: payload.Amount, Currency: strings.ToUpper(payload.Currency), Paid: payload.Status == "SUCCESS", Cancelled: payload.Status == "FAIL" || payload.Status == "EXPIRED" || payload.Status == "CANCEL"}, nil
}

func (g *Gateway) createTribute(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	payload := map[string]any{
		"amount": int64(math.Round(input.Amount * 100)), "currency": strings.ToLower(input.Currency), "title": "Подписка",
		"description": input.Description, "comment": strconv.FormatInt(input.PurchaseID, 10), "customerId": strconv.FormatInt(input.CustomerID, 10),
		"period": "onetime", "successUrl": input.ReturnURL, "failUrl": input.ReturnURL,
	}
	if cfg["shopId"] != "" {
		id, err := strconv.ParseInt(cfg["shopId"], 10, 64)
		if err != nil || id <= 0 {
			return CreatedPayment{}, errors.New("invalid Tribute shop ID")
		}
		payload["shopId"] = id
	}
	raw, _ := json.Marshal(payload)
	var response struct {
		UUID string `json:"uuid"`
		URL  string `json:"paymentUrl"`
	}
	endpoint := strings.TrimRight(firstNonEmpty(cfg["apiUrl"], "https://tribute.tg/api/v1"), "/") + "/shop/orders"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"Api-Key": cfg["apiKey"]}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.UUID == "" || response.URL == "" {
		return CreatedPayment{}, errors.New("Tribute did not return a browser payment link; check that the shop accepts regular payments")
	}
	return CreatedPayment{ExternalID: response.UUID, URL: response.URL}, nil
}

func parseTributeWebhook(cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	signature := strings.ToLower(strings.TrimSpace(headers.Get("trbt-signature")))
	expected := hmacHex(sha256.New, []byte(cfg["apiKey"]), raw)
	if signature == "" || !hmac.Equal([]byte(signature), []byte(expected)) {
		return WebhookPayment{}, errors.New("invalid Tribute webhook signature")
	}
	var payload struct {
		Name    string `json:"name"`
		Payload struct {
			UUID     string `json:"uuid"`
			Amount   int64  `json:"amount"`
			Currency string `json:"currency"`
			Status   string `json:"status"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	if payload.Name != "shop_order" || payload.Payload.UUID == "" || payload.Payload.Amount <= 0 {
		return WebhookPayment{}, errors.New("unsupported Tribute webhook")
	}
	return WebhookPayment{ExternalID: payload.Payload.UUID, Amount: float64(payload.Payload.Amount) / 100, Currency: strings.ToUpper(payload.Payload.Currency), Paid: payload.Payload.Status == "paid"}, nil
}
