package runtimeconfig

import "testing"

func TestBlocksBackgroundSettingsSurviveValidation(t *testing.T) {
	settings := DefaultSettings()
	settings.Appearance.BackgroundMode = "blocks"
	settings.Appearance.Colors["blocksBackground"] = "#101020"
	settings.Appearance.Colors["blocksLeft"] = "#232345"
	settings.Appearance.Colors["blocksRight"] = "#15152a"
	settings.Appearance.BackgroundMotion["blocks"] = BackgroundMotionSettings{Dimming: 25, Speed: 70}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Appearance.BackgroundMode != "blocks" || settings.Appearance.Colors["blocksLeft"] != "#232345" || settings.Appearance.BackgroundMotion["blocks"].Speed != 70 {
		t.Fatal("blocks background customization was lost")
	}
}

func TestExistingAppearanceGainsBlocksDefaults(t *testing.T) {
	settings := DefaultSettings()
	delete(settings.Appearance.BackgroundMotion, "blocks")
	for _, key := range []string{"blocksBackground", "blocksLeft", "blocksRight"} {
		delete(settings.Appearance.Colors, key)
	}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Appearance.BackgroundMode != "animated" || settings.Appearance.Colors["blocksBackground"] != "#0a0a0c" || settings.Appearance.BackgroundMotion["blocks"].Speed != 35 {
		t.Fatal("existing appearance must retain its mode and gain safe blocks defaults")
	}
}
