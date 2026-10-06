package integrations

import (
	"errors"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
)

const ProviderSMTP = "smtp"

func (s *Service) SMTPSettings() (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec := s.records[ProviderSMTP]
	fields := make(map[string]string, len(rec.Config))
	for k, v := range rec.Config {
		fields[k] = v
	}
	return fields, rec.Enabled
}

func ValidateSMTP(fields map[string]string, enabled bool) error {
	for _, value := range fields {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("Поля SMTP не должны содержать переносы строк")
		}
	}
	if !enabled {
		return nil
	}
	port, err := strconv.Atoi(fields["port"])
	if err != nil || port < 1 || port > 65535 {
		return errors.New("Укажите порт от 1 до 65535")
	}
	if strings.TrimSpace(fields["host"]) == "" || strings.ContainsAny(fields["host"], " /\\:@?#") {
		return errors.New("Укажите имя SMTP-сервера без протокола и порта")
	}
	if _, err := mail.ParseAddress(fields["from"]); err != nil {
		return errors.New("Укажите корректный адрес отправителя")
	}
	if fields["user"] == "" || fields["password"] == "" {
		return errors.New("Укажите логин и пароль SMTP")
	}
	if proxy := fields["proxyUrl"]; proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("Прокси должен иметь адрес HTTP или HTTPS")
		}
	}
	return nil
}

func init() {
	definitions = append(definitions, ProviderDefinition{
		ID: ProviderSMTP, Name: "Почта / SMTP", Kind: "email",
		Fields: []FieldDefinition{
			{Key: "host", Label: "SMTP-сервер", Required: true},
			{Key: "port", Label: "Порт", Required: true},
			{Key: "user", Label: "Логин", Required: true},
			{Key: "password", Label: "Пароль", Required: true, Secret: true},
			{Key: "from", Label: "Отправитель", Required: true},
			{Key: "proxyUrl", Label: "HTTP(S)-прокси", Secret: true},
		},
	})
}
