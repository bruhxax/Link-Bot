package runtimeconfig

import (
	"fmt"
	"net/netip"
	"strings"
	"unicode/utf8"
)

type InteractiveSettings struct {
	Enabled         bool     `json:"enabled"`
	Addresses       []string `json:"addresses"`
	ProtectedText   string   `json:"protectedText"`
	UnprotectedText string   `json:"unprotectedText"`
	DurationSeconds int      `json:"durationSeconds"`
}

func DefaultInteractiveSettings() InteractiveSettings {
	return InteractiveSettings{Addresses: []string{}, ProtectedText: "Вы под защитой {brand} — спасибо, что вы с нами!", UnprotectedText: "Вы не под защитой", DurationSeconds: 8}
}
func NormalizeInteractive(s *InteractiveSettings) error {
	defaults := DefaultInteractiveSettings()
	if s.ProtectedText == "" {
		s.ProtectedText = defaults.ProtectedText
	}
	if s.UnprotectedText == "" {
		s.UnprotectedText = defaults.UnprotectedText
	}
	for _, text := range []string{s.ProtectedText, s.UnprotectedText} {
		if strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 300 {
			return fmt.Errorf("текст плашки должен содержать от 1 до 300 символов")
		}
	}
	if s.DurationSeconds < 0 || s.DurationSeconds > 3600 {
		return fmt.Errorf("время показа: от 1 до 3600 секунд или 0 — всегда")
	}
	if len(s.Addresses) > 512 {
		return fmt.Errorf("добавьте не больше 512 IP-адресов или подсетей")
	}
	clean := []string{}
	seen := map[string]bool{}
	for _, raw := range s.Addresses {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if addr, err := netip.ParseAddr(value); err == nil && addr.Zone() == "" {
			value = addr.Unmap().String()
		} else if prefix, err := netip.ParsePrefix(value); err == nil {
			value = prefix.Masked().String()
		} else {
			return fmt.Errorf("некорректный IP-адрес или подсеть: %s", value)
		}
		if !seen[value] {
			seen[value] = true
			clean = append(clean, value)
		}
	}
	s.Addresses = clean
	return nil
}
func (s InteractiveSettings) ContainsIP(value string) bool {
	addr, err := netip.ParseAddr(value)
	if err != nil || addr.Zone() != "" {
		return false
	}
	addr = addr.Unmap()
	for _, raw := range s.Addresses {
		if ip, err := netip.ParseAddr(raw); err == nil && ip.Unmap() == addr {
			return true
		}
		if prefix, err := netip.ParsePrefix(raw); err == nil && prefix.Contains(addr) {
			return true
		}
	}
	return false
}
