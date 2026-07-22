package integrations

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Gateway struct {
	settings   *Service
	httpClient *http.Client
}

type CreatePaymentRequest struct {
	Provider    string
	PurchaseID  int64
	Amount      float64
	Currency    string
	Description string
	CustomerID  int64
	Username    string
	ReturnURL   string
	ClientIP    string
}

type CreatedPayment struct {
	ExternalID string
	URL        string
}

type WebhookPayment struct {
	PurchaseID int64
	ExternalID string
	Amount     float64
	Currency   string
	Paid       bool
	Cancelled  bool
}

func NewGateway(settings *Service) *Gateway {
	return &Gateway{settings: settings, httpClient: &http.Client{Timeout: 12 * time.Second}}
}

func (g *Gateway) Create(ctx context.Context, input CreatePaymentRequest) (CreatedPayment, error) {
	cfg, ok := g.settings.Config(input.Provider)
	if !ok {
		return CreatedPayment{}, fmt.Errorf("интеграция %s не настроена или выключена", input.Provider)
	}
	if input.Currency == "" {
		input.Currency = "RUB"
	}
	switch input.Provider {
	case ProviderLava:
		return g.createLava(ctx, cfg, input)
	case ProviderWata:
		return g.createWata(ctx, cfg, input)
	case ProviderPlatega:
		return g.createPlatega(ctx, cfg, input)
	case ProviderFreeKassa:
		return g.createFreeKassa(cfg, input)
	case ProviderHeleket:
		return g.createHeleket(ctx, cfg, input)
	case ProviderPally:
		return g.createPally(ctx, cfg, input)
	default:
		return CreatedPayment{}, fmt.Errorf("unsupported gateway: %s", input.Provider)
	}
}

func (g *Gateway) HandleWebhook(ctx context.Context, provider string, headers http.Header, raw []byte, form url.Values) (WebhookPayment, error) {
	cfg, ok := g.settings.Config(provider)
	if !ok {
		return WebhookPayment{}, errors.New("integration is disabled")
	}
	switch provider {
	case ProviderLava:
		return parseLavaWebhook(cfg, headers, raw)
	case ProviderWata:
		return g.parseWataWebhook(ctx, cfg, headers, raw)
	case ProviderPlatega:
		return parsePlategaWebhook(cfg, headers, raw)
	case ProviderFreeKassa:
		return parseFreeKassaWebhook(cfg, form)
	case ProviderHeleket:
		return parseHeleketWebhook(cfg, raw)
	case ProviderPally:
		return parsePallyWebhook(cfg, form)
	default:
		return WebhookPayment{}, fmt.Errorf("unknown webhook provider: %s", provider)
	}
}

func (g *Gateway) createLava(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	payload := struct {
		Sum        float64 `json:"sum"`
		OrderID    string  `json:"orderId"`
		ShopID     string  `json:"shopId"`
		HookURL    string  `json:"hookUrl"`
		SuccessURL string  `json:"successUrl"`
		FailURL    string  `json:"failUrl"`
		Expire     int     `json:"expire"`
		Comment    string  `json:"comment"`
	}{input.Amount, strconv.FormatInt(input.PurchaseID, 10), cfg["shopId"], g.settings.WebhookURL(ProviderLava), input.ReturnURL, input.ReturnURL, 60, input.Description}
	raw, _ := json.Marshal(payload)
	signature := hmacHex(sha256.New, []byte(cfg["secretKey"]), raw)
	var response struct {
		Data struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"data"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := g.doJSON(ctx, http.MethodPost, "https://api.lava.ru/business/invoice/create", raw, map[string]string{"Signature": signature}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.Data.ID == "" || response.Data.URL == "" {
		return CreatedPayment{}, fmt.Errorf("LAVA did not return invoice URL: %s", response.Error)
	}
	return CreatedPayment{ExternalID: response.Data.ID, URL: response.Data.URL}, nil
}

func (g *Gateway) createWata(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	payload := map[string]any{
		"amount": input.Amount, "currency": input.Currency, "description": input.Description,
		"orderId": strconv.FormatInt(input.PurchaseID, 10), "successRedirectUrl": input.ReturnURL, "failRedirectUrl": input.ReturnURL,
	}
	raw, _ := json.Marshal(payload)
	var response struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	endpoint := strings.TrimRight(cfg["apiUrl"], "/") + "/links"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"Authorization": "Bearer " + cfg["accessToken"]}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.ID == "" || response.URL == "" {
		return CreatedPayment{}, errors.New("WATA did not return payment link")
	}
	return CreatedPayment{ExternalID: response.ID, URL: response.URL}, nil
}

func (g *Gateway) createPlatega(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	payload := map[string]any{
		"paymentDetails": map[string]any{"amount": input.Amount, "currency": input.Currency},
		"description":    input.Description, "return": input.ReturnURL, "failedUrl": input.ReturnURL,
		"payload":  strconv.FormatInt(input.PurchaseID, 10),
		"metadata": map[string]string{"userId": strconv.FormatInt(input.CustomerID, 10), "userName": input.Username},
	}
	raw, _ := json.Marshal(payload)
	var response struct {
		TransactionID string `json:"transactionId"`
		URL           string `json:"url"`
	}
	endpoint := strings.TrimRight(cfg["apiUrl"], "/") + "/v2/transaction/process"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"X-MerchantId": cfg["merchantId"], "X-Secret": cfg["secretKey"]}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.TransactionID == "" || response.URL == "" {
		return CreatedPayment{}, errors.New("Platega did not return payment link")
	}
	return CreatedPayment{ExternalID: response.TransactionID, URL: response.URL}, nil
}

func (g *Gateway) createFreeKassa(cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	amount := formatAmount(input.Amount)
	orderID := strconv.FormatInt(input.PurchaseID, 10)
	signRaw := strings.Join([]string{cfg["shopId"], amount, cfg["secretWord"], input.Currency, orderID}, ":")
	sign := fmt.Sprintf("%x", md5.Sum([]byte(signRaw)))
	query := url.Values{
		"m": {cfg["shopId"]}, "oa": {amount}, "currency": {input.Currency}, "o": {orderID}, "s": {sign}, "lang": {"ru"},
	}
	return CreatedPayment{ExternalID: orderID, URL: "https://pay.fk.money/?" + query.Encode()}, nil
}

func (g *Gateway) createHeleket(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	payload := map[string]any{
		"amount": formatAmount(input.Amount), "currency": input.Currency, "order_id": strconv.FormatInt(input.PurchaseID, 10),
		"url_return": input.ReturnURL, "url_success": input.ReturnURL, "url_callback": g.settings.WebhookURL(ProviderHeleket),
		"lifetime": 3600, "theme": "dark", "additional_data": strconv.FormatInt(input.CustomerID, 10),
	}
	raw, _ := json.Marshal(payload)
	signSource := base64.StdEncoding.EncodeToString(raw) + cfg["apiKey"]
	sign := fmt.Sprintf("%x", md5.Sum([]byte(signSource)))
	var response struct {
		State   int    `json:"state"`
		Message string `json:"message"`
		Result  struct {
			UUID string `json:"uuid"`
			URL  string `json:"url"`
		} `json:"result"`
	}
	endpoint := strings.TrimRight(cfg["apiUrl"], "/") + "/v1/payment"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"merchant": cfg["merchantId"], "sign": sign}, &response); err != nil {
		return CreatedPayment{}, err
	}
	if response.State != 0 || response.Result.UUID == "" || response.Result.URL == "" {
		return CreatedPayment{}, fmt.Errorf("Heleket: %s", response.Message)
	}
	return CreatedPayment{ExternalID: response.Result.UUID, URL: response.Result.URL}, nil
}

func (g *Gateway) createPally(ctx context.Context, cfg map[string]string, input CreatePaymentRequest) (CreatedPayment, error) {
	orderID := strconv.FormatInt(input.PurchaseID, 10)
	form := url.Values{
		"amount":      {formatAmount(input.Amount)},
		"order_id":    {orderID},
		"description": {input.Description},
		"type":        {"normal"},
		"shop_id":     {cfg["shopId"]},
		"currency_in": {input.Currency},
		"custom":      {orderID},
		"name":        {input.Description},
	}
	endpoint := strings.TrimRight(cfg["apiUrl"], "/") + "/api/v1/bill/create"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return CreatedPayment{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg["apiToken"])
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return CreatedPayment{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return CreatedPayment{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CreatedPayment{}, fmt.Errorf("Pally API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var response struct {
		Success     json.RawMessage `json:"success"`
		LinkPageURL string          `json:"link_page_url"`
		BillID      json.RawMessage `json:"bill_id"`
		Message     string          `json:"message"`
		Error       string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return CreatedPayment{}, fmt.Errorf("decode Pally response: %w", err)
	}
	billID, _ := jsonScalarString(response.BillID)
	if !jsonTruthy(response.Success) || strings.TrimSpace(response.LinkPageURL) == "" || strings.TrimSpace(billID) == "" {
		message := firstNonEmpty(response.Message, response.Error, "Pally did not return payment link")
		return CreatedPayment{}, errors.New(message)
	}
	return CreatedPayment{ExternalID: billID, URL: response.LinkPageURL}, nil
}

func (g *Gateway) doJSON(ctx context.Context, method, endpoint string, body []byte, headers map[string]string, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("payment API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if target != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("decode payment API response: %w", err)
		}
	}
	return nil
}

func parseLavaWebhook(cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	var payload struct {
		InvoiceID string          `json:"invoice_id"`
		OrderID   json.RawMessage `json:"order_id"`
		Status    string          `json:"status"`
		Amount    json.RawMessage `json:"amount"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	signature := strings.TrimSpace(headers.Get("Authorization"))
	signature = strings.TrimSpace(strings.TrimPrefix(signature, "Bearer "))
	canonical, err := sortedJSONObject(raw)
	if err != nil {
		return WebhookPayment{}, err
	}
	expected := hmacHex(sha256.New, []byte(cfg["additionalKey"]), canonical)
	if signature == "" || !hmac.Equal([]byte(strings.ToLower(expected)), []byte(strings.ToLower(signature))) {
		return WebhookPayment{}, errors.New("invalid LAVA webhook signature")
	}
	orderID, err := jsonScalarString(payload.OrderID)
	if err != nil {
		return WebhookPayment{}, err
	}
	purchaseID, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return WebhookPayment{}, err
	}
	amountText, err := jsonScalarString(payload.Amount)
	if err != nil {
		return WebhookPayment{}, err
	}
	amount, err := strconv.ParseFloat(amountText, 64)
	if err != nil {
		return WebhookPayment{}, err
	}
	status := strings.ToLower(payload.Status)
	return WebhookPayment{PurchaseID: purchaseID, ExternalID: payload.InvoiceID, Amount: amount, Currency: "RUB", Paid: status == "success" || status == "paid", Cancelled: status == "cancel" || status == "failed" || status == "expired"}, nil
}

func sortedJSONObject(raw []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result bytes.Buffer
	result.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			result.WriteByte(',')
		}
		encodedKey, _ := json.Marshal(key)
		result.Write(encodedKey)
		result.WriteByte(':')
		var compact bytes.Buffer
		if err := json.Compact(&compact, fields[key]); err != nil {
			return nil, err
		}
		result.Write(compact.Bytes())
	}
	result.WriteByte('}')
	return result.Bytes(), nil
}

func jsonScalarString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", errors.New("missing JSON value")
	}
	var text string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	return strings.TrimSpace(string(raw)), nil
}

func (g *Gateway) parseWataWebhook(ctx context.Context, cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	signature := strings.TrimSpace(headers.Get("X-Signature"))
	if signature == "" {
		return WebhookPayment{}, errors.New("missing WATA signature")
	}
	publicKey, err := g.fetchWataPublicKey(ctx, cfg)
	if err != nil {
		return WebhookPayment{}, err
	}
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return WebhookPayment{}, err
	}
	digest := sha512.Sum512(raw)
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA512, digest[:], sigBytes); err != nil {
		return WebhookPayment{}, errors.New("invalid WATA webhook signature")
	}
	var payload struct {
		ID                string  `json:"id"`
		TransactionStatus string  `json:"transactionStatus"`
		Amount            float64 `json:"amount"`
		Currency          string  `json:"currency"`
		OrderID           string  `json:"orderId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	purchaseID, err := strconv.ParseInt(payload.OrderID, 10, 64)
	if err != nil {
		return WebhookPayment{}, err
	}
	status := strings.ToLower(payload.TransactionStatus)
	return WebhookPayment{PurchaseID: purchaseID, ExternalID: payload.ID, Amount: payload.Amount, Currency: payload.Currency, Paid: status == "paid", Cancelled: status == "declined"}, nil
}

func (g *Gateway) fetchWataPublicKey(ctx context.Context, cfg map[string]string) (*rsa.PublicKey, error) {
	endpoint := strings.TrimRight(cfg["apiUrl"], "/") + "/public-key"
	var response struct {
		Value string `json:"value"`
	}
	if err := g.doJSON(ctx, http.MethodGet, endpoint, nil, map[string]string{"Authorization": "Bearer " + cfg["accessToken"]}, &response); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(response.Value))
	if block == nil {
		return nil, errors.New("invalid WATA public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		if key, ok := parsed.(*rsa.PublicKey); ok {
			return key, nil
		}
	}
	if key, pkcs1Err := x509.ParsePKCS1PublicKey(block.Bytes); pkcs1Err == nil {
		return key, nil
	}
	return nil, errors.New("WATA public key is not RSA")
}

func parsePlategaWebhook(cfg map[string]string, headers http.Header, raw []byte) (WebhookPayment, error) {
	if !hmac.Equal([]byte(headers.Get("X-MerchantId")), []byte(cfg["merchantId"])) || !hmac.Equal([]byte(headers.Get("X-Secret")), []byte(cfg["secretKey"])) {
		return WebhookPayment{}, errors.New("invalid Platega webhook credentials")
	}
	var payload struct {
		ID       string  `json:"id"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
		Status   string  `json:"status"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	// Platega's callback does not include merchant payload, so the local purchase is resolved by external ID.
	status := strings.ToLower(payload.Status)
	return WebhookPayment{ExternalID: payload.ID, Amount: payload.Amount, Currency: payload.Currency, Paid: status == "confirmed", Cancelled: status == "canceled" || status == "chargebacked"}, nil
}

func parseFreeKassaWebhook(cfg map[string]string, form url.Values) (WebhookPayment, error) {
	merchantID, amount, orderID, signature := form.Get("MERCHANT_ID"), form.Get("AMOUNT"), form.Get("MERCHANT_ORDER_ID"), form.Get("SIGN")
	expected := fmt.Sprintf("%x", md5.Sum([]byte(strings.Join([]string{merchantID, amount, cfg["secretWord2"], orderID}, ":"))))
	if merchantID != cfg["shopId"] || !hmac.Equal([]byte(strings.ToLower(expected)), []byte(strings.ToLower(signature))) {
		return WebhookPayment{}, errors.New("invalid FreeKassa webhook signature")
	}
	purchaseID, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return WebhookPayment{}, err
	}
	value, _ := strconv.ParseFloat(amount, 64)
	return WebhookPayment{PurchaseID: purchaseID, ExternalID: form.Get("intid"), Amount: value, Currency: "RUB", Paid: true}, nil
}

func parseHeleketWebhook(cfg map[string]string, raw []byte) (WebhookPayment, error) {
	var payload struct {
		UUID          string `json:"uuid"`
		OrderID       string `json:"order_id"`
		Amount        string `json:"amount"`
		Currency      string `json:"currency"`
		PaymentStatus string `json:"status"`
		Sign          string `json:"sign"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WebhookPayment{}, err
	}
	unsigned := regexp.MustCompile(`,\s*"sign"\s*:\s*"[^"]*"\s*}`).ReplaceAll(raw, []byte("}"))
	if bytes.Equal(unsigned, raw) {
		return WebhookPayment{}, errors.New("invalid Heleket webhook payload")
	}
	signSource := base64.StdEncoding.EncodeToString(unsigned) + cfg["apiKey"]
	expected := fmt.Sprintf("%x", md5.Sum([]byte(signSource)))
	if !hmac.Equal([]byte(strings.ToLower(expected)), []byte(strings.ToLower(payload.Sign))) {
		return WebhookPayment{}, errors.New("invalid Heleket webhook signature")
	}
	purchaseID, err := strconv.ParseInt(payload.OrderID, 10, 64)
	if err != nil {
		return WebhookPayment{}, err
	}
	amount, _ := strconv.ParseFloat(payload.Amount, 64)
	status := strings.ToLower(payload.PaymentStatus)
	return WebhookPayment{PurchaseID: purchaseID, ExternalID: payload.UUID, Amount: amount, Currency: payload.Currency, Paid: status == "paid" || status == "paid_over", Cancelled: status == "cancel" || status == "fail" || status == "wrong_amount" || status == "system_fail" || status == "refund_process" || status == "refund_fail" || status == "refund_paid"}, nil
}

func parsePallyWebhook(cfg map[string]string, form url.Values) (WebhookPayment, error) {
	invoiceID := strings.TrimSpace(form.Get("InvId"))
	amountText := strings.TrimSpace(form.Get("OutSum"))
	signature := strings.TrimSpace(form.Get("SignatureValue"))
	if invoiceID == "" || amountText == "" || signature == "" {
		return WebhookPayment{}, errors.New("invalid Pally webhook payload")
	}
	expected := strings.ToUpper(fmt.Sprintf("%x", md5.Sum([]byte(amountText+":"+invoiceID+":"+cfg["apiToken"]))))
	if !hmac.Equal([]byte(expected), []byte(strings.ToUpper(signature))) {
		return WebhookPayment{}, errors.New("invalid Pally webhook signature")
	}
	purchaseID, err := strconv.ParseInt(invoiceID, 10, 64)
	if err != nil {
		return WebhookPayment{}, err
	}
	amount, err := strconv.ParseFloat(strings.ReplaceAll(amountText, ",", "."), 64)
	if err != nil {
		return WebhookPayment{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(form.Get("Status")))
	return WebhookPayment{
		PurchaseID: purchaseID,
		Amount:     amount,
		Currency:   strings.ToUpper(strings.TrimSpace(form.Get("CurrencyIn"))),
		Paid:       status == "SUCCESS" || status == "OVERPAID",
		Cancelled:  status == "FAIL" || status == "CANCELED" || status == "CANCELLED",
	}, nil
}

func jsonTruthy(raw json.RawMessage) bool {
	value, err := jsonScalarString(raw)
	if err != nil {
		return false
	}
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "true" || value == "1" || value == "success"
}

func decodeJSONNumbers(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func hmacHex(newHash func() hash.Hash, key, raw []byte) string {
	mac := hmac.New(newHash, key)
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

func formatAmount(amount float64) string {
	return strconv.FormatFloat(amount, 'f', 2, 64)
}
