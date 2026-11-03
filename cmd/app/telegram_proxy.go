package main

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// newTelegramProxyClient only changes traffic to the Telegram Bot API.
// net/http sends the target hostname to a SOCKS5h proxy for remote DNS lookup.
func newTelegramProxyClient(raw string) (*http.Client, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	proxyURL, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid TELEGRAM_PROXY_URL")
	}
	proxyURL.Scheme = strings.ToLower(proxyURL.Scheme)
	switch proxyURL.Scheme {
	case "socks5h", "socks5", "http", "https":
	default:
		return nil, errors.New("TELEGRAM_PROXY_URL must use socks5h, socks5, http or https")
	}
	port, err := strconv.Atoi(proxyURL.Port())
	if err != nil || port < 1 || port > 65535 || proxyURL.Hostname() == "" ||
		(proxyURL.Path != "" && proxyURL.Path != "/") || proxyURL.RawQuery != "" || proxyURL.Fragment != "" {
		return nil, errors.New("invalid TELEGRAM_PROXY_URL host or port")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	return &http.Client{Transport: transport, Timeout: time.Minute}, nil
}
