package miniapp

import (
	"math"
	"regexp"
	"strconv"
	"testing"
)

var (
	appearancePresetBlockPattern = regexp.MustCompile(`(?s)\{\s*id:\s*"([^"]+)",\s*name:\s*"([^"]+)",\s*colors:\s*\{(.*?)\n\t\t\},\n\t\},`)
	appearancePresetColorPattern = regexp.MustCompile(`(?m)^\s+([A-Za-z][A-Za-z0-9]*):\s*"(#[0-9a-fA-F]{6})",\s*$`)
)

func TestAppearancePresetsHaveReadableContrast(t *testing.T) {
	raw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	matches := appearancePresetBlockPattern.FindAllStringSubmatch(string(raw), -1)
	if len(matches) < 13 {
		t.Fatalf("appearance presets = %d, want at least 13", len(matches))
	}
	requiredNewPresets := map[string]bool{
		"black-blue":   false,
		"graphite-sky": false,
		"black-amber":  false,
		"slate-violet": false,
		"ivory-navy":   false,
	}
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		id := match[1]
		if _, duplicate := seen[id]; duplicate {
			t.Errorf("duplicate appearance preset id %q", id)
		}
		seen[id] = struct{}{}
		if _, ok := requiredNewPresets[id]; ok {
			requiredNewPresets[id] = true
		}

		colors := make(map[string]string)
		for _, colorMatch := range appearancePresetColorPattern.FindAllStringSubmatch(match[3], -1) {
			colors[colorMatch[1]] = colorMatch[2]
		}
		for _, key := range []string{"background", "surface", "surfaceStrong", "text", "muted", "button", "buttonText", "icon", "accent", "success", "danger"} {
			if colors[key] == "" {
				t.Errorf("preset %q is missing color %q", id, key)
			}
		}

		checks := []struct {
			foreground string
			background string
			minimum    float64
		}{
			{"text", "background", 4.5},
			{"text", "surface", 4.5},
			{"text", "surfaceStrong", 4.5},
			{"muted", "button", 4.5},
			{"muted", "surfaceStrong", 4.5},
			{"buttonText", "button", 4.5},
			{"buttonText", "surfaceStrong", 4.5},
			{"icon", "background", 3},
			{"icon", "surface", 3},
			{"icon", "surfaceStrong", 3},
			{"icon", "button", 3},
			{"accent", "surface", 3},
			{"accent", "surfaceStrong", 3},
			{"success", "surface", 4.5},
			{"success", "surfaceStrong", 4.5},
			{"danger", "surface", 4.5},
			{"danger", "surfaceStrong", 4.5},
		}
		for _, check := range checks {
			if colors[check.foreground] == "" || colors[check.background] == "" {
				continue
			}
			ratio := appearanceContrastRatio(t, colors[check.foreground], colors[check.background])
			if ratio < check.minimum {
				t.Errorf("preset %q contrast %s/%s = %.2f, want at least %.1f", id, check.foreground, check.background, ratio, check.minimum)
			}
		}
	}
	for id, found := range requiredNewPresets {
		if !found {
			t.Errorf("new appearance preset %q is missing", id)
		}
	}
}

func appearanceContrastRatio(t *testing.T, first, second string) float64 {
	t.Helper()
	firstLuminance := appearanceRelativeLuminance(t, first)
	secondLuminance := appearanceRelativeLuminance(t, second)
	if firstLuminance < secondLuminance {
		firstLuminance, secondLuminance = secondLuminance, firstLuminance
	}
	return (firstLuminance + 0.05) / (secondLuminance + 0.05)
}

func appearanceRelativeLuminance(t *testing.T, value string) float64 {
	t.Helper()
	channels := make([]float64, 3)
	for index := range channels {
		parsed, err := strconv.ParseUint(value[1+index*2:3+index*2], 16, 8)
		if err != nil {
			t.Fatalf("parse color %q: %v", value, err)
		}
		channel := float64(parsed) / 255
		if channel <= 0.04045 {
			channels[index] = channel / 12.92
		} else {
			channels[index] = math.Pow((channel+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*channels[0] + 0.7152*channels[1] + 0.0722*channels[2]
}
