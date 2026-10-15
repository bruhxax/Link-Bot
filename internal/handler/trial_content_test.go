package handler

import (
	"testing"

	"link-bot/internal/runtimeconfig"
)

func TestTrialContentButtonsPreserveActionsAndCustomAppearance(t *testing.T) {
	content := runtimeconfig.DefaultSettings().Content
	content.Trial.ActivateButton = runtimeconfig.TelegramButtonSettings{Text: "Try it", Style: "success", IconCustomEmojiID: "12345"}
	content.Trial.ConnectButton = runtimeconfig.TelegramButtonSettings{Text: "My VPN", Style: "primary", IconCustomEmojiID: "67890"}
	content.Commerce.BackButton.Style = "danger"
	before := TrialScreenKeyboard(content, false, "https://my.example.com/mini-app/")
	if len(before) != 2 || before[0][0].CallbackData != CallbackActivateTrial || before[0][0].WebApp != nil || before[0][0].Text != "Try it" || before[0][0].Style != "success" || before[0][0].IconCustomEmojiID != "12345" {
		t.Fatalf("activation button changed: %+v", before)
	}
	for _, miniApp := range []string{"", "https://my.example.com/mini-app/"} {
		after := TrialScreenKeyboard(content, true, miniApp)
		button := after[0][0]
		if button.Text != "My VPN" || button.Style != "primary" || button.IconCustomEmojiID != "67890" {
			t.Fatalf("configured subscription button was lost: %+v", button)
		}
		if miniApp == "" {
			if button.CallbackData != CallbackConnect || button.WebApp != nil {
				t.Fatal("bot-only subscription callback was lost")
			}
		} else if button.WebApp == nil || button.WebApp.URL != miniApp || button.CallbackData != "" {
			t.Fatal("Mini App subscription link was lost")
		}
		if after[1][0].CallbackData != CallbackStart || after[1][0].Style != "danger" {
			t.Fatal("shared back button changed")
		}
	}
}
