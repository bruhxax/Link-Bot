package yookasa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AvailablePaymentMethods reads the merchant settings without creating a payment.
func (c *Client) AvailablePaymentMethods(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ЮKassa: не удалось получить способы оплаты (HTTP %d)", resp.StatusCode)
	}
	var result struct {
		Methods []string `json:"payment_methods"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if result.Methods == nil {
		return nil, fmt.Errorf("ЮKassa не вернула список способов оплаты")
	}
	return result.Methods, nil
}
