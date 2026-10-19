package runtimeconfig

import (
	"net/url"
	"regexp"
	"strings"
)

var paymentLogoUploadPattern = regexp.MustCompile(`^/mini-app/uploads/logo-[0-9a-f]{16}\.(?:png|jpg|webp)$`)

// External logos are loaded as images by the browser, never fetched by the bot.
func ValidPaymentLogoURL(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 2048 || strings.ContainsAny(raw, "\r\n\t\\") {
		return false
	}
	if paymentLogoUploadPattern.MatchString(raw) {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
