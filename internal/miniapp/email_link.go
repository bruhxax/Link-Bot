package miniapp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

type telegramLinkRequest struct {
	IDToken string `json:"idToken"`
}

func (h *Handler) handleLinkTelegramIdentity(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !customer.TelegramIDIsSynthetic {
		h.writeError(w, http.StatusConflict, "telegram_already_linked", "Telegram is already linked")
		return
	}
	linkedEmail, err := h.customerRepository.EmailForCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "telegram_link_failed", "Could not check email account")
		return
	}
	if linkedEmail == "" {
		h.writeError(w, http.StatusConflict, "email_required", "Link email first")
		return
	}
	var request telegramLinkRequest
	if err := h.decodeJSONRequest(w, r, 16<<10, &request); err != nil {
		return
	}
	identity, err := validateTelegramIDToken(r.Context(), request.IDToken, telegramBotID())
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, "invalid_telegram_identity", "Confirm your Telegram account again")
		return
	}
	if config.GetBlockedTelegramIds()[identity.ID] {
		h.writeError(w, http.StatusForbidden, "user_blocked", "Access is blocked")
		return
	}
	linked, err := h.customerRepository.LinkTelegramIdentity(r.Context(), customer.ID, identity.ID, database.NormalizeTelegramUsername(identity.Username))
	switch {
	case errors.Is(err, database.ErrTelegramAlreadyLinked):
		h.writeError(w, http.StatusConflict, "telegram_already_linked", "This Telegram account is already linked to an email")
		return
	case errors.Is(err, database.ErrEmailAccountHasHistory):
		h.writeError(w, http.StatusConflict, "email_account_has_history", "This account has activity; contact support to merge it safely")
		return
	case errors.Is(err, database.ErrTelegramAccountBlocked):
		h.writeError(w, http.StatusForbidden, "user_blocked", "Access is blocked")
		return
	case err != nil:
		slog.Error("link Telegram to email account failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "telegram_link_failed", "Could not link Telegram")
		return
	}
	if linked.IsBlocked || config.GetBlockedTelegramIds()[linked.TelegramID] {
		h.writeError(w, http.StatusForbidden, "user_blocked", "Access is blocked")
		return
	}
	newSession := *sess
	newSession.User.ID = linked.TelegramID
	newSession.User.Username = identity.Username
	newSession.User.FirstName = identity.FirstName
	newSession.User.LastName = identity.LastName
	newSession.User.PhotoURL = identity.Picture
	sessionData, err := createBrowserSessionData(&newSession, config.TelegramToken(), currentMiniAppTime())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "telegram_link_failed", "Could not refresh session")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"sessionData": sessionData}})
}

func (h *Handler) handleStartEmailLink(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if h.runtimeSettings != nil && !h.runtimeSettings.FeatureEnabled("email_auth") {
		h.writeError(w, http.StatusForbidden, "feature_disabled", "Email login is disabled")
		return
	}
	if sess.Provider == sessionProviderEmail {
		h.writeError(w, http.StatusConflict, "email_already_linked", "Email is already linked")
		return
	}
	current, err := h.customerRepository.EmailForCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not check email")
		return
	}
	if current != "" {
		h.writeError(w, http.StatusConflict, "email_already_linked", "Email is already linked")
		return
	}
	settings, configured := h.emailSMTPSettings()
	if !configured {
		h.writeError(w, http.StatusServiceUnavailable, "email_not_configured", "Email is not configured")
		return
	}
	var request emailAuthStartRequest
	if err = h.decodeJSONRequest(w, r, 4096, &request); err != nil {
		return
	}
	email, err := normalizeAuthEmail(request.Email)
	if err != nil || len(request.Password) < 8 || len(request.Password) > 72 {
		h.writeError(w, http.StatusBadRequest, "invalid_email_auth", "Enter a valid email and password of 8–72 characters")
		return
	}
	now := time.Now().UTC()
	if !h.rateLimiter.Allow(fmt.Sprintf("email-link-customer:%d", customer.ID), rateLimitRule{Limit: 5, Window: time.Hour}, now) ||
		!h.rateLimiter.Allow("email-auth-address:"+email, rateLimitRule{Limit: 5, Window: time.Hour}, now) {
		h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
		return
	}
	existing, err := h.customerRepository.EmailPasswordHash(r.Context(), email)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not check email")
		return
	}
	if existing != "" {
		h.writeError(w, http.StatusConflict, "email_already_registered", "This email belongs to another account")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not create challenge")
		return
	}
	id := uuid.New()
	code, err := generateFiveDigitEmailCode()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not create challenge")
		return
	}
	err = h.customerRepository.CreateEmailLinkChallenge(r.Context(), id, customer.ID, email, string(passwordHash), emailCodeHash(id, code), now.Add(10*time.Minute))
	if errors.Is(err, database.ErrEmailChallengeCooldown) {
		h.writeError(w, http.StatusTooManyRequests, "email_code_cooldown", "Wait before requesting another code")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not create challenge")
		return
	}
	if err = sendEmailAuthCode(r.Context(), settings, email, code); err != nil {
		h.customerRepository.DeleteEmailChallenge(r.Context(), id)
		slog.Error("email link delivery failed", "error", err)
		h.writeError(w, http.StatusBadGateway, "email_delivery_failed", "Could not send code")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"challengeId": id.String(), "expiresIn": 600}})
}

func generateFiveDigitEmailCode() (string, error) {
	var random [4]byte
	for {
		if _, err := rand.Read(random[:]); err != nil {
			return "", err
		}
		number := uint32(random[0])<<24 | uint32(random[1])<<16 | uint32(random[2])<<8 | uint32(random[3])
		if number < 4294900000 {
			return fmt.Sprintf("%05d", number%100000), nil
		}
	}
}

func (h *Handler) handleVerifyEmailLink(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if h.runtimeSettings != nil && !h.runtimeSettings.FeatureEnabled("email_auth") {
		h.writeError(w, http.StatusForbidden, "feature_disabled", "Email login is disabled")
		return
	}
	var request emailAuthVerifyRequest
	if err := h.decodeJSONRequest(w, r, 2048, &request); err != nil {
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(request.ChallengeID))
	if err != nil || !fiveDigitCode.MatchString(request.Code) {
		h.writeError(w, http.StatusBadRequest, "invalid_code", "Invalid confirmation code")
		return
	}
	if !h.rateLimiter.Allow(fmt.Sprintf("email-link-verify:%d", customer.ID), rateLimitRule{Limit: 30, Window: time.Minute}, time.Now().UTC()) {
		h.writeError(w, http.StatusTooManyRequests, "too_many_requests", "Too many requests")
		return
	}
	_, err = h.customerRepository.CompleteEmailLinkChallenge(r.Context(), id, customer.ID, emailCodeHash(id, request.Code))
	switch {
	case errors.Is(err, database.ErrEmailCodeInvalid):
		h.writeError(w, http.StatusUnauthorized, "invalid_code", "Invalid confirmation code")
		return
	case errors.Is(err, database.ErrEmailAlreadyRegistered):
		h.writeError(w, http.StatusConflict, "email_already_registered", "This email belongs to another account")
		return
	case err != nil:
		slog.Error("email link verification failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not link email")
		return
	}
	updated, err := h.customerRepository.FindById(r.Context(), customer.ID)
	if err != nil || updated == nil {
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not refresh account")
		return
	}
	payload, err := h.buildBootstrapResponse(r.Context(), sess, updated)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "email_link_failed", "Could not refresh account")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": payload})
}
