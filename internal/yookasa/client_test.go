package yookasa

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNewHTTPClientUsesDirectConnectionWhenProxyIsBlank(t *testing.T) {
	t.Setenv("YOOKASA_PROXY_URL", "  ")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:65535")

	client := newHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("blank YOOKASA_PROXY_URL must use a direct connection")
	}
}

func TestNewHTTPClientUsesConfiguredProxy(t *testing.T) {
	t.Setenv("YOOKASA_PROXY_URL", "http://127.0.0.1:8080")

	client := newHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("configured YOOKASA_PROXY_URL must install a proxy")
	}

	proxyURL, err := transport.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "api.yookassa.ru"}})
	if err != nil {
		t.Fatalf("resolve configured proxy: %v", err)
	}
	if got, want := proxyURL.String(), "http://127.0.0.1:8080"; got != want {
		t.Fatalf("proxy URL = %q, want %q", got, want)
	}
}
