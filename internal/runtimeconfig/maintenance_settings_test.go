package runtimeconfig

import "testing"

func TestMaintenanceKeepsFixedTitleAndCustomText(t *testing.T) {
	defaults := DefaultSettings().Maintenance
	settings := MaintenanceSettings{
		TitleRU:  "Старый заголовок",
		TextRU:   "Вернёмся через час",
		ReasonRU: "Старая причина",
	}

	settings = LocalizeMaintenanceDefaults(settings, "en")
	normalizeMaintenance(&settings, defaults)
	if settings.TitleRU != "Технические работы" {
		t.Fatalf("maintenance title = %q", settings.TitleRU)
	}
	if settings.TextRU != "Вернёмся через час" {
		t.Fatalf("maintenance text = %q", settings.TextRU)
	}
}
