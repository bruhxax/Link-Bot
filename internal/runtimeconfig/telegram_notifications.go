package runtimeconfig

import "fmt"

type TelegramBroadcastPreferencesSettings struct {
	EnabledText       string                 `json:"enabledText"`
	DisabledText      string                 `json:"disabledText"`
	SubscribeButton   TelegramButtonSettings `json:"subscribeButton"`
	UnsubscribeButton TelegramButtonSettings `json:"unsubscribeButton"`
}

func defaultTelegramBroadcastPreferences() TelegramBroadcastPreferencesSettings {
	return TelegramBroadcastPreferencesSettings{
		EnabledText:       "<b>Уведомления</b>\n\nРассылки в Telegram и на почту <b>включены</b>.\nВы можете отключить их и вернуться в любое время.\n\nСообщения об оплате, подписке и поддержке продолжат приходить.",
		DisabledText:      "<b>Уведомления</b>\n\nВы <b>отписались от рассылок</b> в Telegram и на почту.\nВключите их, когда захотите снова получать новости.\n\nСервисные сообщения и обязательные рассылки администратора продолжат приходить.",
		SubscribeButton:   TelegramButtonSettings{Text: "Включить рассылки", Style: "success"},
		UnsubscribeButton: TelegramButtonSettings{Text: "Отключить рассылки", Style: "danger"},
	}
}

func normalizeTelegramNotifications(value *TelegramBroadcastPreferencesSettings, defaults TelegramBroadcastPreferencesSettings) error {
	value.EnabledText = normalizedRequiredText(value.EnabledText, defaults.EnabledText, 3500)
	value.DisabledText = normalizedRequiredText(value.DisabledText, defaults.DisabledText, 3500)
	if err := normalizeTelegramButton(&value.SubscribeButton, defaults.SubscribeButton); err != nil {
		return fmt.Errorf("notifications subscribe button: %w", err)
	}
	if err := normalizeTelegramButton(&value.UnsubscribeButton, defaults.UnsubscribeButton); err != nil {
		return fmt.Errorf("notifications unsubscribe button: %w", err)
	}
	return nil
}
