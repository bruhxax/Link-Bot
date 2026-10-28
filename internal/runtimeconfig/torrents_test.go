package runtimeconfig

import (
	"strings"
	"testing"
)

func TestTorrentTemplatesValidateVariablesAndTelegramHTML(t *testing.T) {
	defaults := DefaultTorrentSettings()
	if err := ValidateTorrentSettings(&defaults); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"{ip}", "{unknown}", "<b>Не закрыто", strings.Repeat("{node}", 30)} {
		s := DefaultTorrentSettings()
		s.UserText = text
		if err := ValidateTorrentSettings(&s); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	current, next := DefaultSettings(), DefaultSettings()
	next.Torrents.UserText = "Нода {node}, срок {duration}"
	next.Content.BrandName = "Other"
	merged, err := MergeSettingsFields(current, next, []string{"torrents"})
	if err != nil || merged.Torrents.UserText != next.Torrents.UserText || merged.Content.BrandName != current.Content.BrandName {
		t.Fatalf("scoped update lost settings: %v", err)
	}
	// Values are substituted once, even when a panel username contains a token.
	result := ReplaceTorrentVariables("{username} / {node}", map[string]string{"username": "{node}", "node": "Frankfurt"}, func(s string) string { return s })
	if result != "{node} / Frankfurt" {
		t.Fatal(result)
	}
}
