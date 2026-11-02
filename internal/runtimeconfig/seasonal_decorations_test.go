package runtimeconfig

import "testing"

func TestSeasonalDecorationsValidateAndKeepDisabledEffects(t *testing.T) {
	legacy := DefaultSettings().Appearance
	legacy.Seasonal = SeasonalDecorationSettings{}
	migrated, err := ValidateThemeAppearance(legacy)
	if err != nil || migrated.Seasonal.Mode != "off" || !migrated.Seasonal.WinterDrifts || !migrated.Seasonal.HalloweenWebs {
		t.Fatalf("older theme lost safe seasonal defaults: %+v, %v", migrated.Seasonal, err)
	}
	appearance := DefaultSettings().Appearance
	appearance.Seasonal.Mode = "winter"
	appearance.Seasonal.Intensity = 72
	appearance.Seasonal.WinterDrifts = false
	validated, err := ValidateThemeAppearance(appearance)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Seasonal.Mode != "winter" || validated.Seasonal.Intensity != 72 || validated.Seasonal.WinterDrifts {
		t.Fatalf("seasonal settings changed: %+v", validated.Seasonal)
	}
	appearance.Seasonal.Mode = "halloween"
	appearance.Seasonal.HalloweenSkulls = true
	appearance.Seasonal.HalloweenWebs = false
	converted, err := ValidateThemeAppearance(appearance)
	if err != nil || !converted.Seasonal.HalloweenWebs || converted.Seasonal.HalloweenSkulls {
		t.Fatalf("old Halloween setting was not migrated to webs: %+v, %v", converted.Seasonal, err)
	}
	appearance.Seasonal.Mode = "unexpected"
	if _, err := ValidateThemeAppearance(appearance); err == nil {
		t.Fatal("accepted an unknown seasonal mode")
	}
	appearance.Seasonal.Mode = "halloween"
	appearance.Seasonal.Intensity = 101
	if _, err := ValidateThemeAppearance(appearance); err == nil {
		t.Fatal("accepted an excessive decoration intensity")
	}
}
