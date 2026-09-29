package miniapp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEmailBrowserSessionRoundTripAndTamperRejection(t *testing.T) {
	const botToken = "123456:test-token"
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	previousTime := currentMiniAppTime
	currentMiniAppTime = func() time.Time { return now }
	defer func() { currentMiniAppTime = previousTime }()

	encoded, err := createBrowserSessionData(&session{Provider: sessionProviderEmail, Email: "User@Example.com", User: telegramUser{ID: 8999999999999999}}, botToken, now)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseAndValidateLoginData(encoded, botToken)
	if err != nil || parsed.Provider != sessionProviderEmail || parsed.Email != "user@example.com" || parsed.User.ID != 8999999999999999 {
		t.Fatalf("email session round trip failed: session=%+v error=%v", parsed, err)
	}
	if _, err := parseAndValidateLoginData(strings.Replace(encoded, "user%40example.com", "other%40example.com", 1), botToken); err == nil {
		t.Fatal("tampered email session was accepted")
	}
}

func TestSMTPHTTPProxyConnect(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			proxyResult := make(chan error, 1)
			go func() {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					proxyResult <- acceptErr
					return
				}
				defer conn.Close()
				request, readErr := http.ReadRequest(bufio.NewReader(conn))
				if readErr != nil {
					proxyResult <- readErr
					return
				}
				if request.Method != http.MethodConnect || request.Host != "smtp.gmail.com:465" || request.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
					proxyResult <- fmt.Errorf("unexpected CONNECT request: method=%s host=%s auth=%s", request.Method, request.Host, request.Header.Get("Proxy-Authorization"))
					return
				}
				_, writeErr := fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n\r\n", status, http.StatusText(status))
				if status == http.StatusOK && writeErr == nil {
					_, writeErr = io.WriteString(conn, "220 SMTP ready\r\n")
				}
				proxyResult <- writeErr
			}()
			conn, err := dialSMTPViaHTTPProxy(context.Background(), &net.Dialer{}, "http://user:pass@"+listener.Addr().String(), "smtp.gmail.com:465")
			if status == http.StatusForbidden {
				if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
					t.Fatalf("expected proxy refusal, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				greeting, readErr := bufio.NewReader(conn).ReadString('\n')
				if readErr != nil || greeting != "220 SMTP ready\r\n" {
					t.Fatalf("SMTP greeting was lost after CONNECT: %q, %v", greeting, readErr)
				}
			}
			if proxyErr := <-proxyResult; proxyErr != nil {
				t.Fatal(proxyErr)
			}
		})
	}
}

func TestEmailSMTPSettingsForGmail(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.gmail.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USER", "sender@gmail.com")
	t.Setenv("SMTP_PASSWORD", "abcd efgh ijkl mnop")
	t.Setenv("SMTP_FROM", "sender@gmail.com")
	settings, ok := emailSMTPSettingsFromEnv()
	if !ok || settings.password != "abcdefghijklmnop" {
		t.Fatalf("Gmail app password was not accepted without display spaces: settings=%+v, configured=%v", emailSMTPSettings{host: settings.host, port: settings.port}, ok)
	}
	root := errors.New("authentication rejected")
	err := emailDeliveryFailure("email_smtp_auth_failed", root)
	var deliveryErr *emailDeliveryError
	if !errors.As(err, &deliveryErr) || deliveryErr.code != "email_smtp_auth_failed" || !errors.Is(err, root) {
		t.Fatalf("SMTP failure lost its category or underlying cause: %v", err)
	}
}

func TestEmailAuthValidationAndCodeBinding(t *testing.T) {
	for _, raw := range []string{"", "not-an-email", "Name <user@example.com>", "user@example.com\r\nX: bad"} {
		if _, err := normalizeAuthEmail(raw); err == nil {
			t.Fatalf("accepted invalid email %q", raw)
		}
	}
	if email, err := normalizeAuthEmail("  User@Example.com  "); err != nil || email != "user@example.com" {
		t.Fatalf("normalization failed: %q, %v", email, err)
	}
	if fiveDigitCode.MatchString("1234") || fiveDigitCode.MatchString("123456") || !fiveDigitCode.MatchString("00123") {
		t.Fatal("five-digit validation failed")
	}
	id := uuid.New()
	if emailCodeHash(id, "12345") == emailCodeHash(id, "12346") || emailCodeHash(id, "12345") == emailCodeHash(uuid.New(), "12345") {
		t.Fatal("code hash is not bound to code and challenge")
	}
}
