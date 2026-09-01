package moynalog

import (
	"bytes"
	"context"
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
