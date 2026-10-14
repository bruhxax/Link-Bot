package devicequota

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"link-bot/internal/remnawave"
)

var usernameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func deviceUsername(parent *remnawave.PanelUser, b *Binding, number int) string {
	owner := usernameChars.ReplaceAllString(parent.Username, "_")
	if owner == "" {
		owner = "user"
	}
	digest := sha256.Sum256([]byte(b.Token.String()))
	suffix := fmt.Sprintf("_device%d_%x", number, digest[:6])
	if len(owner) > 36-len(suffix) {
		owner = owner[:36-len(suffix)]
	}
	return owner + suffix
}

func deviceDescription(parent *remnawave.PanelUser, slot *Slot) string {
	parts := []string{fmt.Sprintf("Устройство №%d", slot.Number)}
	model := strings.Join(strings.Fields(slot.Device.DeviceModel), " ")
	if model == "" {
		model = strings.Join(strings.Fields(slot.Device.Platform), " ")
	}
	if len([]rune(model)) > 48 {
		model = string([]rune(model)[:48])
	}
	if model != "" {
		parts = append(parts, model)
	}
	owner := strings.TrimSpace(parent.Username)
	if owner != "" {
		parts = append(parts, "Подписка: "+owner)
	}
	return strings.Join(parts, " · ")
}
