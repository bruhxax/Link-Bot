package runtimeconfig

import (
	"encoding/json"
	"testing"
)

func validCustomBackground() CustomBackgroundSettings {
	return CustomBackgroundSettings{ID: "bg-test", Name: "Мой фон", URL: "/mini-app/uploads/background-0123456789abcdef.gif", Type: "gif", Fit: "cover", Scale: 150, PositionX: 32, PositionY: 70, Speed: 125, Dimming: 42}
}

func TestCustomBackgroundSettingsRoundTrip(t *testing.T) {
	settings := DefaultSettings()
	settings.Appearance.BackgroundMode = "custom"
	settings.Appearance.CustomBackgrounds = []CustomBackgroundSettings{validCustomBackground()}
	settings.Appearance.ActiveBackground = "bg-test"
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var restored Settings
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err = NormalizeAndValidate(&restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Appearance.CustomBackgrounds) != 1 || restored.Appearance.CustomBackgrounds[0] != validCustomBackground() {
		t.Fatal("custom framing/playback settings were lost")
	}
	restored.Appearance.CustomBackgrounds = nil
	restored.Appearance.ActiveBackground = ""
	if err = NormalizeAndValidate(&restored); err != nil {
		t.Fatalf("removing the last background: %v", err)
	}
}

func TestCustomBackgroundValidation(t *testing.T) {
	for _, change := range []func(*CustomBackgroundSettings){
		func(b *CustomBackgroundSettings) { b.URL = "https://remote.example/bg.gif" },
		func(b *CustomBackgroundSettings) { b.URL = "/mini-app/uploads/../secret.gif" },
		func(b *CustomBackgroundSettings) { b.Type = "video" }, func(b *CustomBackgroundSettings) { b.Scale = 301 },
		func(b *CustomBackgroundSettings) { b.PositionY = -1 }, func(b *CustomBackgroundSettings) { b.Speed = 201 },
		func(b *CustomBackgroundSettings) { b.Dimming = 91 }, func(b *CustomBackgroundSettings) { b.Fit = "stretch" },
	} {
		settings := DefaultSettings()
		item := validCustomBackground()
		change(&item)
		settings.Appearance.CustomBackgrounds = []CustomBackgroundSettings{item}
		if NormalizeAndValidate(&settings) == nil {
			t.Fatalf("invalid background accepted: %+v", item)
		}
	}
	settings := DefaultSettings()
	settings.Appearance.CustomBackgrounds = []CustomBackgroundSettings{validCustomBackground(), validCustomBackground()}
	if NormalizeAndValidate(&settings) == nil {
		t.Fatal("duplicate IDs accepted")
	}
	settings = DefaultSettings()
	settings.Appearance.ActiveBackground = "missing"
	if NormalizeAndValidate(&settings) == nil {
		t.Fatal("missing selection accepted")
	}
}
