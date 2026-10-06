package miniapp

import (
	"context"
	"errors"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"net/http"
	"strings"
	"time"
)

func smtpFields(s emailSMTPSettings) map[string]string {
	return map[string]string{"host": s.host, "port": s.port, "user": s.user, "password": s.password, "from": s.from, "proxyUrl": s.proxyURL}
}
func smtpFromFields(f map[string]string) emailSMTPSettings {
	s := emailSMTPSettings{host: f["host"], port: f["port"], user: f["user"], password: f["password"], from: f["from"], proxyURL: f["proxyUrl"]}
	if s.port == "" {
		s.port = "587"
	}
	if strings.EqualFold(s.host, "smtp.gmail.com") {
		s.password = strings.Join(strings.Fields(s.password), "")
	}
	return s
}
func (h *Handler) emailSMTPSettings() (emailSMTPSettings, bool) {
	if h.integrationSettings != nil {
		fields, enabled := h.integrationSettings.SMTPSettings()
		if fields["host"] != "" {
			s := smtpFromFields(fields)
			return s, enabled && integrations.ValidateSMTP(smtpFields(s), true) == nil
		}
	}
	return emailSMTPSettingsFromEnv()
}
func (h *Handler) adminSMTPView() map[string]any {
	s, enabled := h.emailSMTPSettings()
	source := "env"
	if h.integrationSettings != nil {
		fields, active := h.integrationSettings.SMTPSettings()
		if fields["host"] != "" {
			s, enabled, source = smtpFromFields(fields), active, "admin"
		}
	}
	return map[string]any{"host": s.host, "port": s.port, "user": s.user, "from": s.from, "enabled": enabled, "passwordConfigured": s.password != "", "proxyConfigured": s.proxyURL != "", "source": source}
}

func resolveSMTPSecrets(entered, stored emailSMTPSettings) emailSMTPSettings {
	// Changing the server must not forward saved credentials to another host.
	if strings.EqualFold(entered.host, stored.host) {
		if entered.password == "" {
			entered.password = stored.password
		}
		if entered.proxyURL == "" {
			entered.proxyURL = stored.proxyURL
		}
	}
	return entered
}

func (h *Handler) handleAdminSMTP(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("smtp") {
		h.writeError(w, 403, "forbidden", "Недостаточно прав")
		return
	}
	if h.integrationSettings == nil {
		h.writeError(w, 503, "smtp_unavailable", "Настройки почты недоступны")
		return
	}
	if r.URL.Path == "/api/mini-app/admin/smtp/settings" {
		h.writeJSON(w, 200, map[string]any{"ok": true, "data": h.adminSMTPView()})
		return
	}
	var req struct {
		Host       string `json:"host"`
		Port       string `json:"port"`
		User       string `json:"user"`
		Password   string `json:"password"`
		From       string `json:"from"`
		ProxyURL   string `json:"proxyUrl"`
		Enabled    bool   `json:"enabled"`
		Email      string `json:"email"`
		ClearProxy bool   `json:"clearProxy"`
	}
	if h.decodeJSONRequest(w, r, 16384, &req) != nil {
		h.writeError(w, 400, "invalid_request", "Некорректный запрос")
		return
	}
	stored, _ := h.emailSMTPSettings()
	s := resolveSMTPSecrets(smtpFromFields(map[string]string{"host": strings.TrimSpace(req.Host), "port": strings.TrimSpace(req.Port), "user": strings.TrimSpace(req.User), "password": req.Password, "from": strings.TrimSpace(req.From), "proxyUrl": strings.TrimSpace(req.ProxyURL)}), stored)
	if req.ClearProxy {
		s.proxyURL = ""
	}
	if err := integrations.ValidateSMTP(smtpFields(s), true); err != nil {
		h.writeError(w, 400, "invalid_smtp", err.Error())
		return
	}
	if r.URL.Path == "/api/mini-app/admin/smtp/update" {
		// SMTP's optional proxy can be explicitly cleared; passwords are retained.
		fields := smtpFields(s)
		_, err := h.integrationSettings.Update(r.Context(), integrations.ProviderSMTP, integrations.UpdateInput{Enabled: req.Enabled, Fields: fields}, sess.User.ID)
		if err != nil {
			h.writeError(w, 500, "smtp_save_failed", "Не удалось сохранить почту")
			return
		}
		h.writeJSON(w, 200, map[string]any{"ok": true, "data": h.adminSMTPView()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	var err error
	if strings.HasSuffix(r.URL.Path, "/test") {
		email, normalizeErr := normalizeAuthEmail(req.Email)
		if normalizeErr != nil {
			h.writeError(w, 400, "invalid_email", "Введите адрес получателя тестового письма")
			return
		}
		err = sendSMTPMessage(ctx, s, email, "Проверка почты Link-Bot", "SMTP работает. Это тестовое письмо из панели администратора.")
	} else {
		client, connectErr := connectSMTP(ctx, s)
		err = connectErr
		if err == nil {
			defer client.Close()
			err = client.Noop()
			if err == nil {
				err = client.Quit()
			}
		}
	}
	if err != nil {
		h.writeError(w, 400, "smtp_check_failed", smtpPublicError(err))
		return
	}
	h.writeJSON(w, 200, map[string]any{"ok": true})
}

func smtpPublicError(err error) string {
	var failure *emailDeliveryError
	if errors.As(err, &failure) {
		switch failure.code {
		case "email_smtp_connection_failed":
			return "Не удалось подключиться к SMTP. Проверьте сервер и порт."
		case "email_smtp_tls_failed":
			return "Не удалось установить защищённое соединение. Порт 465 использует TLS, остальные — STARTTLS."
		case "email_smtp_auth_failed":
			return "SMTP отклонил логин или пароль."
		case "email_smtp_sender_failed":
			return "SMTP отклонил адрес отправителя."
		case "email_smtp_recipient_failed":
			return "SMTP отклонил получателя."
		case "email_smtp_proxy_failed":
			return "Не удалось подключиться через прокси."
		}
	}
	return "SMTP не принял проверку или письмо. Проверьте настройки сервера."
}
