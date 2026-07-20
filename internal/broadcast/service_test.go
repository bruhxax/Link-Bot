package broadcast

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"link-bot/internal/database"
)

func TestValidateButtons(t *testing.T) {
	t.Parallel()

	buttons, err := ValidateButtons([]database.BroadcastButton{
		{Type: "url", Text: "Открыть сайт", URL: "https://example.com/news"},
		{
			Type:      "promo",
			Text:      `<tg-emoji emoji-id="5206222720416643915">☺️</tg-emoji> Использовать промокод`,
			Style:     " SUCCESS ",
			PromoCode: " link20 ",
		},
	})
	if err != nil {
		t.Fatalf("ValidateButtons() error = %v", err)
	}
	if len(buttons) != 2 {
		t.Fatalf("ValidateButtons() returned %d buttons, want 2", len(buttons))
	}
	if buttons[1].PromoCode != "LINK20" {
		t.Fatalf("promo code = %q, want LINK20", buttons[1].PromoCode)
	}
	if buttons[1].Text != "Использовать промокод" {
		t.Fatalf("button text = %q, want cleaned label", buttons[1].Text)
	}
	if buttons[1].IconCustomEmojiID != "5206222720416643915" {
		t.Fatalf("premium emoji ID = %q, want extracted ID", buttons[1].IconCustomEmojiID)
	}
	if buttons[1].Style != "success" {
		t.Fatalf("button style = %q, want success", buttons[1].Style)
	}
	if buttons[0].ID == "" || buttons[1].ID == "" || buttons[0].ID == buttons[1].ID {
		t.Fatalf("generated button IDs are not unique: %#v", buttons)
	}
}

func TestValidateButtonsAcceptsEmojiCodeInDedicatedField(t *testing.T) {
	t.Parallel()

	buttons, err := ValidateButtons([]database.BroadcastButton{{
		Type:              "url",
		Text:              "Канал",
		IconCustomEmojiID: `<tg-emoji emoji-id="5206222720416643915">☺️</tg-emoji>`,
		Style:             "primary",
		URL:               "https://example.com",
	}})
	if err != nil {
		t.Fatalf("ValidateButtons() error = %v", err)
	}
	if buttons[0].IconCustomEmojiID != "5206222720416643915" {
		t.Fatalf("premium emoji ID = %q, want extracted ID", buttons[0].IconCustomEmojiID)
	}
}

func TestValidateButtonsRejectsInvalidStyleAndEmoji(t *testing.T) {
	t.Parallel()

	for name, button := range map[string]database.BroadcastButton{
		"style": {Type: "url", Text: "Open", Style: "orange", URL: "https://example.com"},
		"emoji": {Type: "url", Text: "Open", IconCustomEmojiID: "not-an-id", URL: "https://example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ValidateButtons([]database.BroadcastButton{button})
			if !errors.Is(err, ErrInvalidButton) {
				t.Fatalf("ValidateButtons() error = %v, want ErrInvalidButton", err)
			}
		})
	}
}

func TestValidateButtonsRejectsUnsafeURL(t *testing.T) {
	t.Parallel()

	_, err := ValidateButtons([]database.BroadcastButton{{Type: "url", Text: "Open", URL: "http://example.com"}})
	if !errors.Is(err, ErrInvalidButton) {
		t.Fatalf("ValidateButtons() error = %v, want ErrInvalidButton", err)
	}
}

func TestValidateButtonsRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	_, err := ValidateButtons([]database.BroadcastButton{
		{ID: "same", Type: "url", Text: "One", URL: "https://example.com/one"},
		{ID: "same", Type: "url", Text: "Two", URL: "https://example.com/two"},
	})
	if !errors.Is(err, ErrInvalidButton) {
		t.Fatalf("ValidateButtons() error = %v, want ErrInvalidButton", err)
	}
}

func TestBuildKeyboardIncludesPremiumEmojiAndStyle(t *testing.T) {
	t.Parallel()

	markup := buildKeyboard([]database.BroadcastButton{{
		Type:              "url",
		Text:              "Открыть",
		IconCustomEmojiID: "5206222720416643915",
		Style:             "danger",
		URL:               "https://example.com",
	}})
	payload, err := json.Marshal(markup)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	encoded := string(payload)
	if !strings.Contains(encoded, `"icon_custom_emoji_id":"5206222720416643915"`) {
		t.Fatalf("keyboard payload does not include premium emoji: %s", encoded)
	}
	if !strings.Contains(encoded, `"style":"danger"`) {
		t.Fatalf("keyboard payload does not include style: %s", encoded)
	}
}
