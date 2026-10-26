package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"link-bot/internal/database"
)

const laveTestID = "inv_87c8af4c-9200-45af-9bf0-d322e4ac45b5"
const laveTestURL = "https://t.me/laveshka?startapp=lpay_Xyug2fo4Mwuj54Wd"

type laveTestStore struct {
	q   *database.LaveshkaQuote
	err error
}

func (s *laveTestStore) SaveLaveshkaQuote(_ context.Context, q database.LaveshkaQuote) error {
	if s.err != nil {
		return s.err
	}
	s.q = &q
	return nil
}
func (s *laveTestStore) FindLaveshkaQuote(_ context.Context, _ string) (*database.LaveshkaQuote, error) {
	return s.q, s.err
}

type laveTransport func(*http.Request) (*http.Response, error)

func (f laveTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func laveTestGateway(t *testing.T, handler func(*http.Request) (int, string)) (*Gateway, map[string]string, *laveTestStore) {
	t.Helper()
	cfg := map[string]string{"apiKey": "LAVE-12-fixture", "webhookSecret": "whsec_fixture", "rubPerUsd": "100"}
	s := &Service{baseURL: "https://vpn.example", records: map[string]record{ProviderLaveshka: {Enabled: true, WebhookToken: "route", Config: cfg}}}
	g := NewGateway(s)
	store := &laveTestStore{q: &database.LaveshkaQuote{PurchaseID: 42, InvoiceID: laveTestID, MerchantID: 12, RubMinor: 8900, USDMinor: 89}}
	g.laveshkaQuotes = store
	g.httpClient = &http.Client{Transport: laveTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "laveshka.io" || r.Header.Get("X-Api-Key") != cfg["apiKey"] {
			t.Errorf("bad API request %s", r.URL)
		}
		status, body := handler(r)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	return g, cfg, store
}
func laveFixture(status string) laveInvoice {
	return laveInvoice{ID: laveTestID, MerchantID: 12, Status: status, Currency: "USD", FaceAmount: "0.89", OrderID: "42", PayURL: laveTestURL}
}
func laveResponse(invoice laveInvoice) string {
	raw, _ := json.Marshal(map[string]any{"ok": true, "data": invoice})
	return string(raw)
}
func laveEvent(invoice laveInvoice, kind string) ([]byte, http.Header) {
	raw, _ := json.Marshal(map[string]any{"id": "evt_fixture_event", "type": kind, "merchant_id": 12, "data": invoice})
	return raw, laveHeaders(raw, time.Now().Unix())
}
func laveHeaders(raw []byte, ts int64) http.Header {
	timestamp := strconv.FormatInt(ts, 10)
	h := make(http.Header)
	h.Set("X-LavePay-Timestamp", timestamp)
	h.Set("X-LavePay-Signature", "v1="+smerchTestSign("whsec_fixture", timestamp+"."+string(raw)))
	h.Set("X-LavePay-Event-Id", "evt_fixture_event")
	return h
}

func TestLaveshkaExactCurrencyConversion(t *testing.T) {
	for _, tc := range []struct {
		rub  float64
		rate string
		usd  int64
	}{
		{89, "100", 89}, {100, "90", 112}, {0.1, "1", 10}, {100, "90,5", 111}, {1000000, "100", 1000000}, {12.34, "1", 1234}, {0.01, "0.000001", 1000000},
	} {
		t.Run(fmt.Sprintf("%v/%s", tc.rub, tc.rate), func(t *testing.T) {
			rub, usd, err := laveConvert(tc.rub, tc.rate)
			if err != nil || usd != tc.usd || rub != int64(math.Round(tc.rub*100)) {
				t.Fatalf("%d %d %v", rub, usd, err)
			}
		})
	}
	for _, tc := range []struct {
		rub  float64
		rate string
	}{
		{0, "100"}, {-1, "100"}, {math.NaN(), "100"}, {math.Inf(1), "100"}, {89.001, "100"}, {1, "100"}, {1000001, "100"}, {100, "0"}, {100, "-1"}, {100, ""}, {100, "1e2"}, {100, "1/2"}, {100, "1.0000001"},
	} {
		if _, _, err := laveConvert(tc.rub, tc.rate); err == nil {
			t.Errorf("accepted invalid amount/rate %v/%q", tc.rub, tc.rate)
		}
	}
}

func TestLaveshkaCreateUsesUSDAndFreezesQuote(t *testing.T) {
	calls := 0
	g, _, store := laveTestGateway(t, func(r *http.Request) (int, string) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/merchant/v1/invoices" {
			t.Error("wrong create endpoint")
		}
		var in map[string]any
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			t.Fatal("invalid JSON")
		}
		if calls == 1 && (in["amount"] != "0.89" || in["currency"] != "USD" || in["order_id"] != "42" || in["return_url"] != "https://vpn.example/return" || in["ttl"] != float64(3600)) {
			t.Errorf("wrong payload %+v", in)
		}
		if _, ok := in["method"]; ok {
			t.Error("checkout must not use H2H")
		}
		if calls == 1 && len([]rune(in["description"].(string))) != 256 {
			t.Error("description not truncated by runes")
		}
		return 200, laveResponse(laveFixture("active"))
	})
	store.q = nil
	result, err := g.Create(context.Background(), CreatePaymentRequest{Provider: ProviderLaveshka, PurchaseID: 42, Amount: 89, Currency: "RUB", ReturnURL: "https://vpn.example/return", Description: strings.Repeat("Я", 300)})
	if err != nil || result.ExternalID != laveTestID || result.URL != laveTestURL || calls != 1 || store.q == nil || store.q.RubMinor != 8900 || store.q.USDMinor != 89 {
		t.Fatalf("%+v quote=%+v err=%v", result, store.q, err)
	}
	store.err = errors.New("database secret error")
	result, err = g.Create(context.Background(), CreatePaymentRequest{Provider: ProviderLaveshka, PurchaseID: 42, Amount: 89})
	if err == nil || result.URL != "" || strings.Contains(err.Error(), "secret") {
		t.Fatal("must not return an untracked checkout or storage details")
	}
}

func TestLaveshkaCreateRejectsMismatchedInvoicesAndUnsafeLinks(t *testing.T) {
	cases := map[string]func(*laveInvoice){
		"amount": func(i *laveInvoice) { i.FaceAmount = "0.88" }, "merchant": func(i *laveInvoice) { i.MerchantID = 13 }, "order": func(i *laveInvoice) { i.OrderID = "43" }, "currency": func(i *laveInvoice) { i.Currency = "USDT" }, "status": func(i *laveInvoice) { i.Status = "paid" }, "id": func(i *laveInvoice) { i.ID = "../other" },
		"javascript": func(i *laveInvoice) { i.PayURL = "javascript:alert(1)" }, "host": func(i *laveInvoice) { i.PayURL = "https://t.me.evil/laveshka?startapp=lpay_Xyug2fo4Mwuj54Wd" }, "bot": func(i *laveInvoice) { i.PayURL = "https://t.me/other?startapp=lpay_Xyug2fo4Mwuj54Wd" }, "query": func(i *laveInvoice) { i.PayURL = laveTestURL + "&redirect=https://evil.example" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			invoice := laveFixture("active")
			change(&invoice)
			g, _, store := laveTestGateway(t, func(*http.Request) (int, string) { return 200, laveResponse(invoice) })
			store.q = nil
			result, err := g.Create(context.Background(), CreatePaymentRequest{Provider: ProviderLaveshka, PurchaseID: 42, Amount: 89})
			if err == nil || result.URL != "" || store.q != nil {
				t.Fatal("accepted mismatched/untrusted invoice")
			}
		})
	}
}

func TestLaveshkaPaidWebhookVerifiesAPIAndUsesOriginalRUB(t *testing.T) {
	calls := 0
	g, cfg, _ := laveTestGateway(t, func(r *http.Request) (int, string) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/merchant/v1/invoices/"+laveTestID {
			t.Error("bad verify endpoint")
		}
		return 200, laveResponse(laveFixture("paid"))
	})
	// An admin changed the rate after checkout. Even a missing rate must not
	// change the original price of a previously issued invoice.
	cfg["rubPerUsd"] = "200"
	raw, h := laveEvent(laveFixture("paid"), "invoice.paid")
	for n := 0; n < 2; n++ {
		result, err := g.HandleWebhook(context.Background(), ProviderLaveshka, h, raw, nil)
		if err != nil || !result.Paid || result.Cancelled || result.PurchaseID != 42 || result.ExternalID != laveTestID || result.Amount != 89 || result.Currency != "RUB" {
			t.Fatalf("%+v %v", result, err)
		}
	}
	if calls != 2 {
		t.Fatal("every callback needs authenticated API verification")
	}
}

func TestLaveshkaRejectsForgedWebhookAndInvoiceMismatch(t *testing.T) {
	type mutation func(*laveInvoice, http.Header, *laveTestStore)
	cases := map[string]mutation{
		"signature": func(_ *laveInvoice, h http.Header, _ *laveTestStore) {
			h.Set("X-LavePay-Signature", "v1="+strings.Repeat("0", 64))
		},
		"timestamp":    func(_ *laveInvoice, h http.Header, _ *laveTestStore) { h.Set("X-LavePay-Timestamp", "1") },
		"event header": func(_ *laveInvoice, h http.Header, _ *laveTestStore) { h.Set("X-LavePay-Event", "invoice.expired") },
		"event id":     func(_ *laveInvoice, h http.Header, _ *laveTestStore) { h.Set("X-LavePay-Event-Id", "evt_other") },
		"amount":       func(i *laveInvoice, _ http.Header, _ *laveTestStore) { i.FaceAmount = "0.90" },
		"merchant":     func(i *laveInvoice, _ http.Header, _ *laveTestStore) { i.MerchantID = 13 },
		"currency":     func(i *laveInvoice, _ http.Header, _ *laveTestStore) { i.Currency = "USDT" },
		"order":        func(i *laveInvoice, _ http.Header, _ *laveTestStore) { i.OrderID = "042" },
		"id":           func(i *laveInvoice, _ http.Header, _ *laveTestStore) { i.ID = "inv_../evil" },
		"quote":        func(_ *laveInvoice, _ http.Header, s *laveTestStore) { s.q.MerchantID = 13 },
		"status":       func(i *laveInvoice, _ http.Header, _ *laveTestStore) { i.Status = "active" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			g, _, store := laveTestGateway(t, func(*http.Request) (int, string) { calls++; return 200, laveResponse(laveFixture("paid")) })
			invoice := laveFixture("paid")
			headers := make(http.Header)
			change(&invoice, headers, store)
			raw, h := laveEvent(invoice, "invoice.paid")
			for k, v := range headers {
				h[k] = v
			}
			_, err := g.HandleWebhook(context.Background(), ProviderLaveshka, h, raw, nil)
			if !errors.Is(err, ErrInvalidLaveshkaWebhook) || calls != 0 {
				t.Fatalf("err=%v api calls=%d", err, calls)
			}
		})
	}
	for _, status := range []string{"active", "refunded", "paid"} {
		t.Run("api "+status, func(t *testing.T) {
			invoice := laveFixture(status)
			if status == "paid" {
				invoice.FaceAmount = "0.90"
			}
			g, _, _ := laveTestGateway(t, func(*http.Request) (int, string) { return 200, laveResponse(invoice) })
			raw, h := laveEvent(laveFixture("paid"), "invoice.paid")
			result, err := g.HandleWebhook(context.Background(), ProviderLaveshka, h, raw, nil)
			if result.Paid || status == "active" && (err == nil || errors.Is(err, ErrInvalidLaveshkaWebhook)) || status == "refunded" && (err != nil || !result.Ignored) || status == "paid" && !errors.Is(err, ErrInvalidLaveshkaWebhook) {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestLaveshkaWebhookIgnoredEventsAndRetryableFailures(t *testing.T) {
	for _, kind := range []string{"invoice.underpaid", "invoice.refunded", "webhook.test", "deposit.completed"} {
		t.Run(kind, func(t *testing.T) {
			g, _, _ := laveTestGateway(t, func(*http.Request) (int, string) { t.Fatal("ignored event queried API"); return 0, "" })
			raw, h := laveEvent(laveFixture("active"), kind)
			result, err := g.HandleWebhook(context.Background(), ProviderLaveshka, h, raw, nil)
			if err != nil || !result.Ignored || result.Paid {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
	for _, kind := range []string{"expired", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			g, _, _ := laveTestGateway(t, func(*http.Request) (int, string) { return 200, laveResponse(laveFixture(kind)) })
			raw, h := laveEvent(laveFixture(kind), "invoice."+kind)
			result, err := g.HandleWebhook(context.Background(), ProviderLaveshka, h, raw, nil)
			if err != nil || !result.Cancelled || result.Paid {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
	for _, failure := range []string{"missing quote", "storage", "api", "bad envelope"} {
		t.Run(failure, func(t *testing.T) {
			g, _, store := laveTestGateway(t, func(*http.Request) (int, string) {
				if failure == "api" {
					return 503, `{"secret":"LAVE-12-fixture"}`
				}
				return 200, `{"ok":false,"error":{"message":"LAVE-12-fixture"}}`
			})
			if failure == "missing quote" {
				store.q = nil
			}
			if failure == "storage" {
				store.err = errors.New("secret")
			}
			raw, h := laveEvent(laveFixture("paid"), "invoice.paid")
			result, err := g.HandleWebhook(context.Background(), ProviderLaveshka, h, raw, nil)
			if err == nil || result.Paid || errors.Is(err, ErrInvalidLaveshkaWebhook) || strings.Contains(err.Error(), "fixture") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
	raw, _ := laveEvent(laveFixture("paid"), "invoice.paid")
	for _, delta := range []int64{-301, 61} {
		g, _, _ := laveTestGateway(t, func(*http.Request) (int, string) { t.Fatal("stale event queried API"); return 0, "" })
		if _, err := g.HandleWebhook(context.Background(), ProviderLaveshka, laveHeaders(raw, time.Now().Unix()+delta), raw, nil); !errors.Is(err, ErrInvalidLaveshkaWebhook) {
			t.Fatal("accepted stale/future signature")
		}
	}
}
