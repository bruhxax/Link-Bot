package runtimeconfig

import "testing"

func TestNotificationsContentMigratesAndPreservesVisibilityAndCustomization(t *testing.T) {
	s := DefaultSettings()
	s.Content.Notifications = TelegramBroadcastPreferencesSettings{}
	s.Content.StartMenu.NotificationsButton = OptionalTelegramButtonSettings{}
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	if s.Content.StartMenu.NotificationsButton.Enabled || s.Content.Notifications.EnabledText == "" || s.Content.Notifications.SubscribeButton.Text == "" {
		t.Fatal("old settings migrated incorrectly")
	}
	s.Content.StartMenu.NotificationsButton = OptionalTelegramButtonSettings{Enabled: true, TelegramButtonSettings: TelegramButtonSettings{Text: "My notices", IconCustomEmojiID: "123456789", Style: "primary"}}
	s.Content.Notifications.DisabledText = "<b>Custom opt-out message</b>"
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	localized := LocalizeTelegramDefaults(s.Content, "en")
	if !localized.StartMenu.NotificationsButton.Enabled || localized.StartMenu.NotificationsButton.Text != "My notices" || localized.Notifications.DisabledText != s.Content.Notifications.DisabledText {
		t.Fatal("customization lost")
	}
	s.Content.StartMenu.NotificationsButton.Enabled = false
	if err := NormalizeAndValidate(&s); err != nil || s.Content.StartMenu.NotificationsButton.Enabled {
		t.Fatalf("hiding button lost: %v", err)
	}
	s.Content.StartMenu.NotificationsButton.Style = "purple"
	if err := NormalizeAndValidate(&s); err == nil {
		t.Fatal("invalid button style accepted")
	}
}
