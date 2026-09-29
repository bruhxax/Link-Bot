package miniapp

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

var fiveDigitCode = regexp.MustCompile(`^[0-9]{5}$`)

type emailAuthStartRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Mode     string `json:"mode"`
}

type emailAuthVerifyRequest struct {
	ChallengeID string `json:"challengeId"`
	Code        string `json:"code"`
}

func normalizeAuthEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return "", errors.New("invalid email")
	}
	return email, nil
}

func emailCodeHash(id uuid.UUID, code string) string {
	mac := hmac.New(sha256.New, []byte(config.TelegramToken()))
	mac.Write([]byte(id.String() + ":" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Handler) handleStartEmailAuth(w http.ResponseWriter, r *http.Request) {
	setAPIHeaders(w)
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !hasAcceptedContentType(r.Header.Get("Content-Type"), []string{"application/json"}) {
		h.writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON required")
		return
	}
	settings, configured := emailSMTPSettingsFromEnv()
	if !configured {
		h.writeError(w, http.StatusServiceUnavailable, "email_not_configured", "Email login is not configured")
		return
	}
	now := time.Now().UTC()
	if !h.rateLimiter.Allow("email-auth-start:"+publicRequestIP(r), rateLimitRule{Limit: 8, Window: time.Hour}, now) {
		w.Header().Set("Retry-After", "3600")
		h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
		return
	}
	var request emailAuthStartRequest
	if err := h.decodeJSONRequest(w, r, 4096, &request); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	email, err := normalizeAuthEmail(request.Email)
	if err != nil || (request.Mode != "login" && request.Mode != "register") || len(request.Password) < 8 || len(request.Password) > 72 {
		h.writeError(w, http.StatusBadRequest, "invalid_email_auth", "Enter a valid email and a password of 8–72 characters")
		return
	}
	if !h.rateLimiter.Allow("email-auth-address:"+email, rateLimitRule{Limit: 5, Window: time.Hour}, now) {
		w.Header().Set("Retry-After", "3600")
		h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
		return
	}
	passwordHash, err := h.customerRepository.EmailPasswordHash(r.Context(), email)
	if err != nil {
		slog.Error("email auth lookup failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to start email login")
		return
	}
	challengePasswordHash := ""
	if request.Mode == "login" {
		if passwordHash == "" || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.Password)) != nil {
			h.writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
			return
		}
	} else {
		if passwordHash != "" {
			h.writeError(w, http.StatusConflict, "email_already_registered", "Email is already registered")
			return
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to start registration")
			return
		}
		challengePasswordHash = string(hash)
	}
	id := uuid.New()
	var random [4]byte
	if _, err = rand.Read(random[:]); err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to create a code")
		return
	}
	// Rejection sampling avoids modulo bias in the five-digit code.
	number := uint32(random[0])<<24 | uint32(random[1])<<16 | uint32(random[2])<<8 | uint32(random[3])
	for number >= 4294900000 {
		if _, err = rand.Read(random[:]); err != nil {
			h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to create a code")
			return
		}
		number = uint32(random[0])<<24 | uint32(random[1])<<16 | uint32(random[2])<<8 | uint32(random[3])
	}
	code := fmt.Sprintf("%05d", number%100000)
	err = h.customerRepository.CreateEmailChallenge(r.Context(), id, email, request.Mode, challengePasswordHash, emailCodeHash(id, code), now.Add(10*time.Minute))
	if errors.Is(err, database.ErrEmailChallengeCooldown) {
		w.Header().Set("Retry-After", "60")
		h.writeError(w, http.StatusTooManyRequests, "email_code_cooldown", "Wait before requesting another code")
		return
	}
	if err != nil {
		slog.Error("email challenge creation failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to create a code")
		return
	}
	if err = sendEmailAuthCode(r.Context(), settings, email, code); err != nil {
		h.customerRepository.DeleteEmailChallenge(r.Context(), id)
		slog.Error("email auth delivery failed", "error", err)
		code := "email_delivery_failed"
		var deliveryErr *emailDeliveryError
		if errors.As(err, &deliveryErr) {
			code = deliveryErr.code
		}
		h.writeError(w, http.StatusBadGateway, code, "Unable to send the confirmation code")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"challengeId": id.String(), "expiresIn": 600}})
}

func (h *Handler) handleVerifyEmailAuth(w http.ResponseWriter, r *http.Request) {
	setAPIHeaders(w)
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !hasAcceptedContentType(r.Header.Get("Content-Type"), []string{"application/json"}) {
		h.writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON required")
		return
	}
	if !h.rateLimiter.Allow("email-auth-verify:"+publicRequestIP(r), rateLimitRule{Limit: 30, Window: time.Minute}, time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
		return
	}
	var request emailAuthVerifyRequest
	if err := h.decodeJSONRequest(w, r, 2048, &request); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	id, err := uuid.Parse(request.ChallengeID)
	if err != nil || !fiveDigitCode.MatchString(request.Code) {
		h.writeError(w, http.StatusBadRequest, "invalid_code", "Invalid confirmation code")
		return
	}
	customer, err := h.customerRepository.CompleteEmailChallenge(r.Context(), id, emailCodeHash(id, request.Code), h.language())
	if errors.Is(err, database.ErrEmailCodeInvalid) {
		h.writeError(w, http.StatusUnauthorized, "invalid_code", "Invalid confirmation code")
		return
	}
	if errors.Is(err, database.ErrEmailAlreadyRegistered) {
		h.writeError(w, http.StatusConflict, "email_already_registered", "Email is already registered")
		return
	}
	if err != nil || customer == nil {
		slog.Error("email auth verification failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to complete email login")
		return
	}
	if customer.IsBlocked || config.GetBlockedTelegramIds()[customer.TelegramID] {
		h.writeError(w, http.StatusForbidden, "user_blocked", "Access is blocked")
		return
	}
	// The address is looked up from the verified challenge, never trusted from the request.
	email, err := h.customerRepository.EmailForCustomer(r.Context(), customer.ID)
	if err != nil || email == "" {
		h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to complete email login")
		return
	}
	sessionData, err := createBrowserSessionData(&session{Provider: sessionProviderEmail, Email: email, User: telegramUser{ID: customer.TelegramID, FirstName: strings.Split(email, "@")[0], LanguageCode: h.language()}}, config.TelegramToken(), currentMiniAppTime())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_auth_failed", "Unable to complete email login")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"sessionData": sessionData}})
}

type emailSMTPSettings struct {
	host, port, user, password, from, proxyURL string
}

type emailDeliveryError struct {
	code string
	err  error
}

func (e *emailDeliveryError) Error() string { return e.code + ": " + e.err.Error() }
func (e *emailDeliveryError) Unwrap() error { return e.err }

func emailDeliveryFailure(code string, err error) error {
	return &emailDeliveryError{code: code, err: err}
}

func emailSMTPSettingsFromEnv() (emailSMTPSettings, bool) {
	s := emailSMTPSettings{host: strings.TrimSpace(os.Getenv("SMTP_HOST")), port: strings.TrimSpace(os.Getenv("SMTP_PORT")), user: strings.TrimSpace(os.Getenv("SMTP_USER")), password: os.Getenv("SMTP_PASSWORD"), from: strings.TrimSpace(os.Getenv("SMTP_FROM")), proxyURL: strings.TrimSpace(os.Getenv("SMTP_PROXY_URL"))}
	// Google displays app passwords in groups of four characters for readability.
	if strings.EqualFold(s.host, "smtp.gmail.com") {
		s.password = strings.Join(strings.Fields(s.password), "")
	}
	if s.port == "" {
		s.port = "587"
	}
	_, portErr := strconv.Atoi(s.port)
	_, fromErr := mail.ParseAddress(s.from)
	return s, s.host != "" && s.user != "" && s.password != "" && fromErr == nil && portErr == nil
}

func sendEmailAuthCode(ctx context.Context, settings emailSMTPSettings, recipient, code string) error {
	err := sendEmailAuthCodeOnce(ctx, settings, recipient, code)
	var deliveryErr *emailDeliveryError
	if err != nil && strings.EqualFold(settings.host, "smtp.gmail.com") && settings.port == "587" && errors.As(err, &deliveryErr) && (deliveryErr.code == "email_smtp_connection_failed" || deliveryErr.code == "email_smtp_proxy_failed") {
		settings.port = "465"
		if retryErr := sendEmailAuthCodeOnce(ctx, settings, recipient, code); retryErr != nil {
			return fmt.Errorf("Gmail SMTP port 587 failed (%v); port 465 failed: %w", err, retryErr)
		}
		return nil
	}
	return err
}

func sendEmailAuthCodeOnce(ctx context.Context, settings emailSMTPSettings, recipient, code string) error {
	address := net.JoinHostPort(settings.host, settings.port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if settings.proxyURL != "" {
		conn, err = dialSMTPViaHTTPProxy(ctx, dialer, settings.proxyURL, address)
		if err != nil {
			return emailDeliveryFailure("email_smtp_proxy_failed", err)
		}
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return emailDeliveryFailure("email_smtp_connection_failed", err)
		}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if settings.port == "465" {
		secureConn := tls.Client(conn, &tls.Config{ServerName: settings.host, MinVersion: tls.VersionTLS12})
		if err = secureConn.HandshakeContext(ctx); err != nil {
			return emailDeliveryFailure("email_smtp_tls_failed", err)
		}
		conn = secureConn
	}
	client, err := smtp.NewClient(conn, settings.host)
	if err != nil {
		return emailDeliveryFailure("email_smtp_connection_failed", err)
	}
	defer client.Close()
	if settings.port != "465" {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return emailDeliveryFailure("email_smtp_tls_failed", errors.New("SMTP server does not support STARTTLS"))
		}
		if err = client.StartTLS(&tls.Config{ServerName: settings.host, MinVersion: tls.VersionTLS12}); err != nil {
			return emailDeliveryFailure("email_smtp_tls_failed", err)
		}
	}
	if err = client.Auth(smtp.PlainAuth("", settings.user, settings.password, settings.host)); err != nil {
		return emailDeliveryFailure("email_smtp_auth_failed", err)
	}
	from, _ := mail.ParseAddress(settings.from)
	if err = client.Mail(from.Address); err != nil {
		return emailDeliveryFailure("email_smtp_sender_failed", err)
	}
	if err = client.Rcpt(recipient); err != nil {
		return emailDeliveryFailure("email_smtp_recipient_failed", err)
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	subject := mime.QEncoding.Encode("utf-8", "Код подтверждения входа")
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nКод подтверждения: %s\r\nОн действует 10 минут. Если вы не запрашивали вход, проигнорируйте письмо.\r\n", settings.from, recipient, subject, code)
	if _, err = io.WriteString(writer, message); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

type smtpProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *smtpProxyConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func dialSMTPViaHTTPProxy(ctx context.Context, dialer *net.Dialer, rawProxyURL, target string) (net.Conn, error) {
	proxy, err := url.Parse(rawProxyURL)
	if err != nil || (proxy.Scheme != "http" && proxy.Scheme != "https") || proxy.Hostname() == "" || proxy.Host == "" || proxy.Path != "" || proxy.RawQuery != "" || proxy.Fragment != "" {
		return nil, errors.New("invalid SMTP_PROXY_URL; expected an HTTP or HTTPS proxy URL")
	}
	proxyAddress := proxy.Host
	if proxy.Port() == "" {
		port := "80"
		if proxy.Scheme == "https" {
			port = "443"
		}
		proxyAddress = net.JoinHostPort(proxy.Hostname(), port)
	}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("proxy connection failed: %w", err)
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if proxy.Scheme == "https" {
		secureConn := tls.Client(conn, &tls.Config{ServerName: proxy.Hostname(), MinVersion: tls.VersionTLS12})
		if err = secureConn.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("proxy TLS failed: %w", err)
		}
		conn = secureConn
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if proxy.User != nil {
		username := proxy.User.Username()
		password, _ := proxy.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	}
	if err = request.Write(conn); err != nil {
		return nil, fmt.Errorf("proxy CONNECT request failed: %w", err)
	}
	reader := bufio.NewReader(conn)
	response, readErr := http.ReadResponse(reader, request)
	if readErr != nil {
		err = readErr
		return nil, fmt.Errorf("proxy CONNECT response failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("proxy CONNECT to SMTP was refused: HTTP %d", response.StatusCode)
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return &smtpProxyConn{Conn: conn, reader: reader}, nil
}
