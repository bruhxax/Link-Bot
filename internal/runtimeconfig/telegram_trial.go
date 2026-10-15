package runtimeconfig

import "fmt"

type TelegramTrialSettings struct {
	Banner          string                 `json:"banner"`
	Text            string                 `json:"text"`
	ActivateButton  TelegramButtonSettings `json:"activateButton"`
	ActivatedBanner string                 `json:"activatedBanner"`
	ActivatedText   string                 `json:"activatedText"`
	ConnectButton   TelegramButtonSettings `json:"connectButton"`
}

func defaultTelegramTrialSettings() TelegramTrialSettings {
	// Preserve the previously bundled trial screens when upgrading old settings.
	return TelegramTrialSettings{
		Banner:          "https://i.pinimg.com/1200x/77/0b/b4/770bb497c2c4d67cddb22a85f7ea8576.jpg",
		Text:            "<tg-emoji emoji-id='5255813559572508065'>☺️</tg-emoji> <b>Пробный период</b>\n\nВы можете бесплатно протестировать сервис перед покупкой.\n\n<tg-emoji emoji-id='5258419835922030550'>☺️</tg-emoji> Ограничено по времени.",
		ActivateButton:  TelegramButtonSettings{Text: "🔥 Активировать бесплатно"},
		ActivatedBanner: "https://i.pinimg.com/736x/1f/59/d4/1f59d41b36f22b661914be96a5f863ba.jpg",
		ActivatedText:   "<tg-emoji emoji-id='5357069174512303778'>☺️</tg-emoji> <b>Пробный период активирован!</b>\n\n<tg-emoji emoji-id='5258132936401624790'>☺️</tg-emoji> Оцените скорость и стабильность сервиса.",
		ConnectButton:   TelegramButtonSettings{Text: "⚙️ Подписка"},
	}
}

func normalizeTelegramTrial(value *TelegramTrialSettings, defaults TelegramTrialSettings) error {
	value.Text = normalizedRequiredText(value.Text, defaults.Text, 3500)
	value.ActivatedText = normalizedRequiredText(value.ActivatedText, defaults.ActivatedText, 3500)
	for _, banner := range []struct {
		name  string
		value *string
	}{{"trial banner", &value.Banner}, {"activated trial banner", &value.ActivatedBanner}} {
		if err := normalizeOptionalAsset(banner.value, banner.name); err != nil {
			return err
		}
	}
	if err := normalizeTelegramButton(&value.ActivateButton, defaults.ActivateButton); err != nil {
		return fmt.Errorf("trial activation button: %w", err)
	}
	if err := normalizeTelegramButton(&value.ConnectButton, defaults.ConnectButton); err != nil {
		return fmt.Errorf("trial subscription button: %w", err)
	}
	return nil
}
