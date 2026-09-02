package moynalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCreateIncomeReturnsReceiptUUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/lkfl":
			var auth AuthRequest
			if err := json.NewDecoder(r.Body).Decode(&auth); err != nil {
				t.Fatalf("decode auth request: %v", err)
			}
			if got := auth.DeviceInfo.SourceDeviceId; len(got) != 21 || got == "*" {
				t.Fatalf("unexpected source device ID: %q", got)
			}
			if auth.DeviceInfo.MetaDetails.UserAgent == "" || r.Header.Get("User-Agent") == "" {
				t.Fatal("auth request must include browser device details")
			}
			_, _ = w.Write([]byte(`{"token":"test-token"}`))
		case "/income":
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			raw, _ := io.ReadAll(r.Body)
			if !bytes.Contains(raw, []byte("+03:00")) {
				t.Errorf("receipt timestamps must use Moscow wall-clock time: %s", raw)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"approvedReceiptUuid":"receipt-123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "123456789012", "secret")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	receipt, err := client.CreateIncome(context.Background(), 299, "VPN-подписка")
	if err != nil {
		t.Fatalf("CreateIncome: %v", err)
	}
	if receipt.ReceiptUUID() != "receipt-123" {
		t.Fatalf("unexpected receipt UUID: %q", receipt.ReceiptUUID())
	}
}

func TestNormalizeBaseURLUpgradesLegacyFNSAPIAddress(t *testing.T) {
	got := normalizeBaseURL("https://lknpd.nalog.ru/api/")
	if got != "https://lknpd.nalog.ru/api/v1" {
		t.Fatalf("normalizeBaseURL = %q", got)
	}
}

func TestStableDeviceIDIsDeterministicPerTaxpayer(t *testing.T) {
	first := stableDeviceID("123456789012")
	second := stableDeviceID("123456789012")
	other := stableDeviceID("987654321098")
	if len(first) != 21 || first != second || first == other {
		t.Fatalf("unexpected stable device IDs: %q %q %q", first, second, other)
	}
}

func TestCreateIncomeDoesNotRetryUncertainResponse(t *testing.T) {
	var incomeRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/lkfl" {
			_, _ = w.Write([]byte(`{"token":"test-token"}`))
			return
		}
		if r.URL.Path == "/income" {
			incomeRequests.Add(1)
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"temporary error"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "123456789012", "secret")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.CreateIncome(context.Background(), 299, "VPN-подписка")
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("expected ErrUncertain, got %v", err)
	}
	if got := incomeRequests.Load(); got != 1 {
		t.Fatalf("income endpoint called %d times; want exactly once", got)
	}
}
