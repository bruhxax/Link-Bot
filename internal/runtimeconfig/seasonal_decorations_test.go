package runtimeconfig

import "testing"

func TestSeasonalDecorationsValidateAndKeepDisabledEffects(t *testing.T) {
	legacy := DefaultSettings().Appearance
	legacy.Seasonal = SeasonalDecorationSettings{}
	migrated, err := ValidateThemeAppearance(legacy)
	if err != nil || migrated.Seasonal.Mode != "off" || !migrated.Seasonal.WinterGarland {
		t.Fatalf("older theme lost safe seasonal defaults: %+v, %v", migrated.Seasonal, err)
	}
	appearance := DefaultSettings().Appearance
	appearance.Seasonal.Mode = "winter"
	appearance.Seasonal.Intensity = 72
	appearance.Seasonal.WinterGarland = false
	validated, err := ValidateThemeAppearance(appearance)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Seasonal.Mode != "winter" || validated.Seasonal.Intensity != 72 || validated.Seasonal.WinterGarland {
		t.Fatalf("seasonal settings changed: %+v", validated.Seasonal)
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
