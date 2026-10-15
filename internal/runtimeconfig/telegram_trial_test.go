package runtimeconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrialContentMigratesOldSettingsAndPreservesEditedMessages(t *testing.T) {
	settings := DefaultSettings()
	settings.Content.Trial = TelegramTrialSettings{}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Content.Trial.Banner == "" || settings.Content.Trial.ActivatedBanner == "" || settings.Content.Trial.ActivatedText == "" {
		t.Fatal("old trial screens lost their default text or banners")
	}
	settings.Content.Trial.Text = "<b>Custom offer</b>"
	settings.Content.Trial.ActivatedText = "<b>Custom success</b>"
	settings.Content.Trial.Banner = ""
	settings.Content.Trial.ActivatedBanner = ""
	settings.Content.Trial.ActivateButton = TelegramButtonSettings{Text: "Test now", Style: "success", IconCustomEmojiID: "123456789"}
	settings.Localization.Language = "en"
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var restored Settings
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := NormalizeAndValidate(&restored); err != nil {
		t.Fatal(err)
	}
	if restored.Content.Trial != settings.Content.Trial {
		t.Fatal("saved trial text, cleared banners or styled buttons changed after reload")
	}
	if restored.Content.Trial.Banner != "" || restored.Content.Trial.ActivatedBanner != "" {
		t.Fatal("cleared trial banners were restored")
	}
	localized := LocalizeTelegramDefaults(restored.Content, "fa")
	if localized.Trial.Text != "<b>Custom offer</b>" || localized.Trial.ActivatedText != "<b>Custom success</b>" || localized.Trial.ActivateButton.Text != "Test now" {
		t.Fatal("language change overwrote custom trial content")
	}
}

func TestTrialContentRejectsUnsafeBannersAndButtonStyles(t *testing.T) {
	for _, invalid := range []string{"banner", "activatedBanner", "button"} {
		t.Run(invalid, func(t *testing.T) {
			settings := DefaultSettings()
			switch invalid {
			case "banner":
				settings.Content.Trial.Banner = "javascript:alert(1)"
			case "activatedBanner":
				settings.Content.Trial.ActivatedBanner = "file:///etc/passwd"
			case "button":
				settings.Content.Trial.ConnectButton.Style = "arbitrary"
			}
			if err := NormalizeAndValidate(&settings); err == nil {
				t.Fatal("invalid trial content accepted")
			}
		})
	}
}

func TestTrialBundledDefaultsFollowLanguage(t *testing.T) {
	content := DefaultSettings().Content
	for _, language := range []string{"en", "fa", "ru"} {
		content = LocalizeTelegramDefaults(content, language)
		if content.Trial.Text == "" || content.Trial.ActivatedText == "" || content.Trial.ActivateButton.Text == "" {
			t.Fatalf("missing trial defaults for %s", language)
		}
		if language == "ru" && !strings.Contains(content.Trial.ActivatedText, "Пробный период активирован") {
			t.Fatal("Russian defaults were not restored after language changes")
		}
	}
}
