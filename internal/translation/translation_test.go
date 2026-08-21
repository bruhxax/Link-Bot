package translation

import (
	"testing"
)

func TestInitTranslationsFallsBackToBundledRussian(t *testing.T) {
	manager := &Manager{
		translations:    make(map[string]Translation),
		defaultLanguage: "en",
	}

	if err := manager.InitTranslations(t.TempDir(), "ru"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}
	if manager.defaultLanguage != "ru" {
		t.Fatalf("default language = %q, want ru", manager.defaultLanguage)
	}
	if len(manager.translations["ru"]) == 0 {
		t.Fatal("bundled Russian translation was not loaded")
	}
}

func TestInitTranslationsUsesAvailableFallback(t *testing.T) {
	manager := &Manager{
		translations:    make(map[string]Translation),
		defaultLanguage: "en",
	}

	if err := manager.InitTranslations(t.TempDir(), "missing"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}
	if manager.defaultLanguage != "ru" {
		t.Fatalf("default language = %q, want ru", manager.defaultLanguage)
	}
}

func TestGlobalPersianLanguageUsesBundledTranslation(t *testing.T) {
	manager := &Manager{translations: make(map[string]Translation), defaultLanguage: "ru"}
	if err := manager.InitTranslations(t.TempDir(), "ru"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}
	manager.SetActiveLanguage("farsi")
	if got := manager.ActiveLanguage(); got != "fa" {
		t.Fatalf("ActiveLanguage() = %q, want fa", got)
	}
	if got := manager.GetText("ru", "web_app_button_text"); got != "حساب کاربری" {
		t.Fatalf("global Persian translation = %q", got)
	}
	start, connect := CommandDescriptions("fa-IR")
	if start == "" || connect == "" {
		t.Fatalf("Persian command descriptions are empty: %q %q", start, connect)
	}
}
