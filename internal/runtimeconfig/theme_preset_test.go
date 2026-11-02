package runtimeconfig

import (
	"encoding/json"
	"testing"
)

func TestThemeAppearanceRoundTrip(t *testing.T) {
	appearance := DefaultSettings().Appearance
	appearance.BackgroundMode = "twinkle"
	appearance.Colors["accent"] = "#123abc"
	appearance.BottomNavStyle = "capsule"
	appearance.Glass = true
	raw, err := json.Marshal(appearance)
	if err != nil {
		t.Fatal(err)
	}
	var imported AppearanceSettings
	if err := json.Unmarshal(raw, &imported); err != nil {
		t.Fatal(err)
	}
	imported, err = ValidateThemeAppearance(imported)
	if err != nil {
		t.Fatal(err)
	}
	if imported.BackgroundMode != "twinkle" || imported.Colors["accent"] != "#123abc" || imported.BottomNavStyle != "capsule" || !imported.Glass || imported.Landing.Colors == nil {
		t.Fatal("theme appearance was not preserved")
	}
	if _, err := ValidateThemeAppearance(AppearanceSettings{}); err == nil {
		t.Fatal("empty theme file accepted")
	}
}
