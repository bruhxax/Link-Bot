package broadcast

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"link-bot/internal/database"
)

func TestValidateHTMLAcceptsTelegramMarkup(t *testing.T) {
	t.Parallel()

	message := `<b>Летнее предложение</b>\n<a href="https://example.com/deal">Открыть</a>\n<blockquote expandable>Условия</blockquote>\n<tg-emoji emoji-id="52764212364458059956">😊</tg-emoji>`
	if err := ValidateHTML(message); err != nil {
		t.Fatalf("ValidateHTML() error = %v", err)
	}
	if !looksLikeBroadcastHTML(message) {
		t.Fatal("Telegram HTML was not detected")
	}
	if preview := broadcastHTMLText(message); strings.Contains(preview, "<b>") || !strings.Contains(preview, "Летнее предложение") {
		t.Fatalf("HTML preview = %q", preview)
	}
}

func TestValidateHTMLRejectsBrokenOrUnsafeMarkup(t *testing.T) {
	t.Parallel()

	for name, message := range map[string]string{
		"unclosed":    `<b>Текст`,
		"unsafe tag":  `<script>alert(1)</script>`,
		"unsafe link": `<a href="javascript:alert(1)">Открыть</a>`,
		"bad nesting": `<b><i>Текст</b></i>`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateHTML(message); !errors.Is(err, ErrInvalidHTML) {
				t.Fatalf("ValidateHTML(%q) error = %v, want ErrInvalidHTML", message, err)
			}
		})
	}
}

func TestMessageKindAcceptsCaptionedUnknownTelegramMessage(t *testing.T) {
	t.Parallel()
	if kind := messageKind(&models.Message{Caption: "Подпись нового типа сообщения"}); kind != "text" {
		t.Fatalf("messageKind() = %q, want text", kind)
	}
}

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

func TestValidateButtonsAcceptsMiniAppDestinations(t *testing.T) {
	t.Parallel()

	types := []string{"main", "reviews", "referrals", "login_methods", "support", "devices"}
	for _, buttonType := range types {
		t.Run(buttonType, func(t *testing.T) {
			t.Parallel()
			buttons, err := ValidateButtons([]database.BroadcastButton{{Type: buttonType, Text: "Open"}})
			if err != nil {
				t.Fatalf("ValidateButtons(%q) error = %v", buttonType, err)
			}
			if len(buttons) != 1 || buttons[0].Type != buttonType {
				t.Fatalf("unexpected buttons for %q: %+v", buttonType, buttons)
			}
		})
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
