package moynalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrAuth   = errors.New("authentication error")
	ErrClient = errors.New("client error")
	// ErrUncertain means the request could have reached FNS. It must not be
	// retried automatically because the income endpoint is not idempotent.
	ErrUncertain = errors.New("receipt result is uncertain")
	moscowZone   = time.FixedZone("MSK", 3*60*60)
)

type Client struct {
	httpClient *http.Client
	baseURL    string

	username string
	password string

	token atomic.Value

	authMu       sync.Mutex
	authInFlight bool
	authCond     *sync.Cond
}

func NewClient(baseURL, username, password string) (*Client, error) {
	c := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username: strings.TrimSpace(username),
		password: password,
	}
	c.authCond = sync.NewCond(&c.authMu)

	c.token.Store("")

	if err := c.authenticate(); err != nil {
		return nil, fmt.Errorf("initial auth failed: %w", err)
	}

	return c, nil
}

func (c *Client) authenticate() error {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	for c.authInFlight {
		c.authCond.Wait()
	}

	if c.token.Load().(string) != "" {
		return nil
	}

	c.authInFlight = true

	c.authMu.Unlock()
	err := c.authenticateOnce()
	c.authMu.Lock()

	c.authInFlight = false
	c.authCond.Broadcast()

	return err
}

func (c *Client) authenticateOnce() error {
	authURL := fmt.Sprintf("%s/auth/lkfl", c.baseURL)

	reqBody, err := json.Marshal(AuthRequest{
		Username: c.username,
		Password: c.password,
		DeviceInfo: DeviceInfo{
			SourceDeviceId: "*",
			SourceType:     "WEB",
			AppVersion:     "1.0.0",
		},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", authURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d: %s", ErrAuth, resp.StatusCode, b)
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return err
	}

	c.token.Store(authResp.Token)
	return nil
}

func (c *Client) CreateIncome(ctx context.Context, amount float64, comment string) (*CreateIncomeResponse, error) {
	resp, err := c.createIncomeOnce(ctx, amount, comment)
	if !errors.Is(err, ErrAuth) {
		return resp, err
	}

	// A 401/403 response proves that the income was rejected, so one retry
	// after refreshing the token is safe.
	c.token.Store("")
	if authErr := c.authenticate(); authErr != nil {
		return nil, fmt.Errorf("reauth failed: %w", authErr)
	}
	return c.createIncomeOnce(ctx, amount, comment)
}

func (c *Client) createIncomeOnce(ctx context.Context, amount float64, comment string) (*CreateIncomeResponse, error) {
	incomeURL := fmt.Sprintf("%s/income", c.baseURL)
	// FNS interprets the wall-clock part as Moscow time even when an offset is
	// present, so always put Moscow clock digits on the wire.
	now := time.Now().In(moscowZone)

	reqBody, err := json.Marshal(CreateIncomeRequest{
		OperationTime: now,
		RequestTime:   now,
		Services: []Service{
			{
				Name:     comment,
				Amount:   amount,
				Quantity: 1,
			},
		},
		TotalAmount: fmt.Sprintf("%.2f", amount),
		Client: IncomeClient{
			IncomeType: "FROM_INDIVIDUAL",
		},
		PaymentType:                     "CASH",
		IgnoreMaxTotalIncomeRestriction: false,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", incomeURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	token := c.token.Load().(string)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUncertain, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w: status %d", ErrAuth, resp.StatusCode)

	case resp.StatusCode >= 500:
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrUncertain, resp.StatusCode, b)

	case resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated:
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrClient, resp.StatusCode, b)
	}

	var incomeResp CreateIncomeResponse
	if err := json.NewDecoder(resp.Body).Decode(&incomeResp); err != nil {
		return nil, fmt.Errorf("%w: decode successful response: %v", ErrUncertain, err)
	}

	return &incomeResp, nil
}
