package runtimeconfig

import "testing"

func TestNotificationFormsMigrateAndPreserveCustomization(t *testing.T) {
	s := DefaultSettings()
	s.Content.StartMenu.NotificationsOffButton = TelegramButtonSettings{}
	s.Content.StartMenu.NotificationsButton = OptionalTelegramButtonSettings{Enabled: true, TelegramButtonSettings: TelegramButtonSettings{Text: "Уведомления"}}
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	if !s.Content.StartMenu.NotificationsButton.Enabled || s.Content.StartMenu.NotificationsButton.Text != "Уведомления: вкл." || s.Content.StartMenu.NotificationsButton.Style != "success" || s.Content.StartMenu.NotificationsOffButton.Style != "danger" {
		t.Fatalf("legacy forms: %+v", s.Content.StartMenu)
	}
	s.Content.StartMenu.NotificationsButton.Text = "My on form"
	s.Content.StartMenu.NotificationsButton.Style = "primary"
	s.Content.StartMenu.NotificationsOffButton.Text = "My off form"
	s.Content.StartMenu.NotificationsOffButton.IconCustomEmojiID = "123456789"
	s.Content.StartMenu.NotificationsOffButton.Style = ""
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	localized := LocalizeTelegramDefaults(s.Content, "en")
	if localized.StartMenu.NotificationsButton.Text != "My on form" || localized.StartMenu.NotificationsOffButton.Text != "My off form" || localized.StartMenu.NotificationsOffButton.Style != "" || localized.StartMenu.NotificationsOffButton.IconCustomEmojiID != "123456789" {
		t.Fatal("custom appearance lost")
	}
	s.Content.StartMenu.NotificationsButton.Enabled = false
	if err := NormalizeAndValidate(&s); err != nil || s.Content.StartMenu.NotificationsButton.Enabled {
		t.Fatalf("hiding button lost: %v", err)
	}
	s.Content.StartMenu.NotificationsOffButton.Style = "purple"
	if err := NormalizeAndValidate(&s); err == nil {
		t.Fatal("invalid off form style accepted")
	}
}
