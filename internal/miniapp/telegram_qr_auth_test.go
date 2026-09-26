package miniapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/webauth"
)

func TestTelegramQRLoginPollsDoNotShareBudgetBehindNAT(t *testing.T) {
	handler := &Handler{webLogin: webauth.NewService(webauth.DefaultTTL), rateLimiter: newRequestRateLimiter()}
	first, _ := handler.webLogin.Create(time.Now().UTC())
	second, _ := handler.webLogin.Create(time.Now().UTC())
	poll := func(challenge webauth.Challenge) int {
		body, _ := json.Marshal(telegramQRLoginStatusRequest{ID: challenge.ID, Secret: challenge.Secret})
		request := httptest.NewRequest(http.MethodPost, "/api/mini-app/auth/telegram/qr/status", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "192.0.2.1:12345"
		response := httptest.NewRecorder()
		handler.handleTelegramQRLoginStatus(response, request)
		return response.Code
	}
	for i := 0; i < 90; i++ {
		if status := poll(first); status != http.StatusOK {
			t.Fatalf("first browser poll %d: %d", i, status)
		}
	}
	if status := poll(first); status != http.StatusTooManyRequests {
		t.Fatalf("poll flood status=%d", status)
	}
	for i := 0; i < 90; i++ {
		if status := poll(second); status != http.StatusOK {
			t.Fatalf("other browser shares budget at poll %d: %d", i, status)
		}
	}
}

func TestTelegramQRLoginReloadReusesPendingChallenge(t *testing.T) {
	previousBotURL := config.BotURL()
	config.SetBotURL("https://t.me/link_bot")
	defer config.SetBotURL(previousBotURL)
	handler := &Handler{webLogin: webauth.NewService(webauth.DefaultTTL), rateLimiter: newRequestRateLimiter()}
	var cookie *http.Cookie
	var firstID string
	for i := 0; i < 75; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/mini-app/auth/telegram/qr/start", bytes.NewBufferString(`{}`))
		request.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.handleStartTelegramQRLogin(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("reload %d: status %d", i, response.Code)
		}
		var payload struct {
			Data struct {
				ID string `json:"id"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstID = payload.Data.ID
			cookie = response.Result().Cookies()[0]
		} else if payload.Data.ID != firstID {
			t.Fatal("reload created a different challenge")
		}
	}
}

func TestTelegramQRLoginStartKeepsBrowserSecretOutOfQRCode(t *testing.T) {
	previousBotURL := config.BotURL()
	config.SetBotURL("https://t.me/link_bot")
	defer config.SetBotURL(previousBotURL)

	handler := &Handler{webLogin: webauth.NewService(webauth.DefaultTTL), rateLimiter: newRequestRateLimiter()}
	request := httptest.NewRequest(http.MethodPost, "/api/mini-app/auth/telegram/qr/start", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.handleStartTelegramQRLogin(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
			URL    string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK || payload.Data.ID == "" || payload.Data.Secret == "" || payload.Data.URL == "" {
		t.Fatalf("incomplete response: %#v", payload)
	}
	if strings.Contains(payload.Data.URL, payload.Data.ID) || strings.Contains(payload.Data.URL, payload.Data.Secret) {
		t.Fatal("QR URL must not expose the browser challenge id or secret")
	}

	parsed, err := url.Parse(payload.Data.URL)
	if err != nil {
		t.Fatalf("parse QR URL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "t.me" || parsed.Path != "/link_bot" {
		t.Fatalf("unexpected Telegram URL: %s", payload.Data.URL)
	}
	if _, ok := webauth.ApprovalToken(parsed.Query().Get("start")); !ok {
		t.Fatalf("missing web login start parameter: %s", payload.Data.URL)
	}

	statusBody, _ := json.Marshal(telegramQRLoginStatusRequest{ID: payload.Data.ID, Secret: payload.Data.Secret})
	statusRequest := httptest.NewRequest(http.MethodPost, "/api/mini-app/auth/telegram/qr/status", bytes.NewReader(statusBody))
	statusRequest.Header.Set("Content-Type", "application/json")
	statusResponse := httptest.NewRecorder()
	handler.handleTelegramQRLoginStatus(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `"status":"pending"`) {
		t.Fatalf("pending status = %d, body = %s", statusResponse.Code, statusResponse.Body.String())
	}
}

func TestTelegramQRLoginStatusRejectsWrongBrowserSecret(t *testing.T) {
	previousBotURL := config.BotURL()
	config.SetBotURL("https://t.me/link_bot")
	defer config.SetBotURL(previousBotURL)

	handler := &Handler{webLogin: webauth.NewService(webauth.DefaultTTL), rateLimiter: newRequestRateLimiter()}
	startRequest := httptest.NewRequest(http.MethodPost, "/api/mini-app/auth/telegram/qr/start", bytes.NewBufferString(`{}`))
	startRequest.Header.Set("Content-Type", "application/json")
	startResponse := httptest.NewRecorder()
	handler.handleStartTelegramQRLogin(startResponse, startRequest)

	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(startResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	statusBody, _ := json.Marshal(telegramQRLoginStatusRequest{ID: payload.Data.ID, Secret: "wrong-secret"})
	statusRequest := httptest.NewRequest(http.MethodPost, "/api/mini-app/auth/telegram/qr/status", bytes.NewReader(statusBody))
	statusRequest.Header.Set("Content-Type", "application/json")
	statusResponse := httptest.NewRecorder()
	handler.handleTelegramQRLoginStatus(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusForbidden || !strings.Contains(statusResponse.Body.String(), `"code":"qr_login_forbidden"`) {
		t.Fatalf("status = %d, body = %s", statusResponse.Code, statusResponse.Body.String())
	}
}
