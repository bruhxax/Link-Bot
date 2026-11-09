package runtimeconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInteractiveValidationAndMatching(t *testing.T) {
	cfg := DefaultInteractiveSettings()
	cfg.Enabled = true
	cfg.Addresses = []string{"203.0.113.8", "::ffff:203.0.113.8", "2001:db8::7", "198.51.100.19/24"}
	if err := NormalizeInteractive(&cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Addresses) != 3 {
		t.Fatalf("dedup %v", cfg.Addresses)
	}
	for _, value := range []string{"203.0.113.8", "::ffff:203.0.113.8", "2001:db8::7", "198.51.100.200"} {
		if !cfg.ContainsIP(value) {
			t.Fatalf("missing %s", value)
		}
	}
	for _, value := range []string{"203.0.113.80", "2001:db8::8", "198.51.101.200", "unknown"} {
		if cfg.ContainsIP(value) {
			t.Fatalf("false match %s", value)
		}
	}
	raw, _ := json.Marshal(cfg)
	var restored InteractiveSettings
	if err := json.Unmarshal(raw, &restored); err != nil || !restored.ContainsIP("203.0.113.8") {
		t.Fatal("restore failed")
	}
	for _, bad := range []string{"example.com", "https://1.2.3.4", "1.2.3.4:80", "fe80::1%eth0", "1.2.3.999", "1.2.3.4/33"} {
		cfg := DefaultInteractiveSettings()
		cfg.Addresses = []string{bad}
		if NormalizeInteractive(&cfg) == nil {
			t.Fatalf("invalid address accepted %s", bad)
		}
	}
	for _, change := range []func(*InteractiveSettings){func(c *InteractiveSettings) { c.DurationSeconds = -1 }, func(c *InteractiveSettings) { c.DurationSeconds = 3601 }, func(c *InteractiveSettings) { c.ProtectedText = strings.Repeat("я", 301) }, func(c *InteractiveSettings) { c.UnprotectedText = "   " }} {
		cfg := DefaultInteractiveSettings()
		change(&cfg)
		if NormalizeInteractive(&cfg) == nil {
			t.Fatal("invalid settings accepted")
		}
	}
}
func TestInteractiveScopedSavePreservesOtherSettings(t *testing.T) {
	current := DefaultSettings()
	next := DefaultSettings()
	next.Interactive.Enabled = true
	next.Content.BrandName = "should not overwrite"
	merged, err := MergeSettingsFields(current, next, []string{"interactive"})
	if err != nil {
		t.Fatal(err)
	}
	if !merged.Interactive.Enabled || merged.Content.BrandName != current.Content.BrandName {
		t.Fatal("scope escaped")
	}
	if err = NormalizeAndValidate(&merged); err != nil {
		t.Fatal(err)
	}
}
