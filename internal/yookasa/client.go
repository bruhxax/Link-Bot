package yookasa

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"link-bot/internal/config"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type YookasaAPI interface {
	CreatePayment(ctx context.Context, request PaymentRequest, idempotencyKey string) (*Payment, error)
	GetPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error)
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	authHeader string
	email      string
}

func NewClient(baseURL, shopID, secretKey string) *Client {
	return NewConfiguredClient(baseURL, shopID, secretKey, config.YookasaEmail())
}

func NewConfiguredClient(baseURL, shopID, secretKey, email string) *Client {
	auth := fmt.Sprintf("%s:%s", shopID, secretKey)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))

	return &Client{
		httpClient: newHTTPClient(),
		baseURL:    baseURL,
		authHeader: fmt.Sprintf("Basic %s", encodedAuth),
		email:      strings.TrimSpace(email),
	}
}

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// YooKassa must use the VPS connection unless its dedicated proxy is set.
	// Do not inherit HTTP_PROXY/HTTPS_PROXY from the process environment.
	transport.Proxy = nil
	transport.MaxIdleConns = 20
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 30 * time.Second
	transport.ResponseHeaderTimeout = 6 * time.Second
	transport.TLSHandshakeTimeout = 5 * time.Second
	if proxyRawURL := strings.TrimSpace(os.Getenv("YOOKASA_PROXY_URL")); proxyRawURL != "" {
		if proxyURL, err := url.Parse(proxyRawURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			log.Printf("Invalid YOOKASA_PROXY_URL, using direct YooKassa connection: %v", err)
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
	}
}

func (c *Client) CreateInvoice(ctx context.Context, amount int, month int, customerId int64, purchaseId int64, returnURL string) (*Payment, error) {
	rub := Amount{
		Value:    strconv.Itoa(amount),
		Currency: "RUB",
	}

	description := formatSubscriptionDescription(month)
	receipt := &Receipt{
		Customer: &Customer{
			Email: c.email,
		},
		Items: []Item{
			{
				VatCode:        1,
				Quantity:       "1",
				Description:    description,
				Amount:         rub,
				PaymentSubject: "payment",
				PaymentMode:    "full_payment",
			},
		},
	}

	metaData := paymentMetadata(ctx, customerId, purchaseId)

	paymentRequest := NewPaymentRequest(
		rub,
		returnURL,
		description,
		receipt,
		metaData,
	)
	paymentRequest.SavePaymentMethod = false

	payment, err := c.CreatePayment(ctx, paymentRequest, uuid.New().String())
	if err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return payment, nil
}

func (c *Client) ChargeSavedPaymentMethod(ctx context.Context, amount int, month int, customerId int64, purchaseId int64, paymentMethodID uuid.UUID) (*Payment, error) {
	rub := Amount{
		Value:    strconv.Itoa(amount),
		Currency: "RUB",
	}

	description := formatSubscriptionDescription(month)
	receipt := &Receipt{
		Customer: &Customer{
			Email: c.email,
		},
		Items: []Item{
			{
				VatCode:        1,
				Quantity:       "1",
				Description:    description,
				Amount:         rub,
				PaymentSubject: "payment",
				PaymentMode:    "full_payment",
			},
		},
	}

	metaData := paymentMetadata(ctx, customerId, purchaseId)
	metaData["autoPayment"] = true

	request := PaymentRequest{
		Amount:            rub,
		Capture:           true,
		Description:       description,
		PaymentMethodID:   &paymentMethodID,
		SavePaymentMethod: false,
		Receipt:           receipt,
		Metadata:          metaData,
	}

	payment, err := c.CreatePayment(ctx, request, uuid.New().String())
	if err != nil {
		return nil, fmt.Errorf("failed to create recurring payment: %w", err)
	}

	return payment, nil
}

func (c *Client) CreatePayment(ctx context.Context, request PaymentRequest, idempotencyKey string) (*Payment, error) {
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payment request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		payment, retry, requestErr := c.createPaymentOnce(ctx, reqBody, idempotencyKey)
		if requestErr == nil {
			return payment, nil
		}
		lastErr = requestErr
		if !retry || ctx.Err() != nil || attempt > 0 {
			break
		}

		c.httpClient.CloseIdleConnections()
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
}

func (c *Client) createPaymentOnce(ctx context.Context, reqBody []byte, idempotencyKey string) (*Payment, bool, error) {
	paymentURL := fmt.Sprintf("%s/payments", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", paymentURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Idempotence-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, true, fmt.Errorf("error while reading invoice resp: %w", err)
		}
		retry := resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout
		return nil, retry, fmt.Errorf("API return error. Status: %d, Body: %s", resp.StatusCode, string(body))
	}

	var payment Payment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		return nil, true, fmt.Errorf("failed to decode response: %w", err)
	}

	return &payment, false, nil
}

func (c *Client) GetPayment(ctx context.Context, paymentID uuid.UUID) (*Payment, error) {
	paymentURL := fmt.Sprintf("%s/payments/%s", c.baseURL, paymentID)

	var payment *Payment

	maxRetries := 5
	baseDelay := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", paymentURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", c.authHeader)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			payment = new(Payment)
			if err := json.NewDecoder(resp.Body).Decode(payment); err != nil {
				return nil, fmt.Errorf("failed to decode response: %w", err)
			}
			return payment, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryDelay := baseDelay * time.Duration(1<<attempt)
			log.Printf("Received 429 Too Many Requests. Retrying in %v...", retryDelay)
			time.Sleep(retryDelay)
			continue
		}

		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("exceeded maximum retries due to 429 Too Many Requests")
}

func paymentMetadata(ctx context.Context, customerId int64, purchaseId int64) map[string]any {
	metaData := map[string]any{
		"customerId": customerId,
		"purchaseId": purchaseId,
	}

	if username, ok := contextStringValue(ctx, "username"); ok {
		metaData["username"] = strings.TrimSpace(username)
	}
	if telegramName, ok := contextStringValue(ctx, "telegramName"); ok {
		metaData["telegramName"] = strings.TrimSpace(telegramName)
	}

	return metaData
}

func contextStringValue(ctx context.Context, key string) (string, bool) {
	if ctx == nil {
		return "", false
	}

	value := ctx.Value(key)
	if value == nil {
		return "", false
	}

	switch v := value.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	default:
		return "", true
	}
}

func formatSubscriptionDescription(month int) string {
	switch month {
	case 1:
		return "Подписка на 1 месяц"
	case 3:
		return "Подписка на 3 месяца"
	case 6:
		return "Подписка на 6 месяцев"
	case 12:
		return "Подписка на 12 месяцев"
	default:
		return fmt.Sprintf("Подписка на %d мес.", month)
	}
}
