package integrations

import (
	"strings"
	"testing"
)

func TestParseP2PConfigNormalizesDestinations(t *testing.T) {
	settings, err := ParseP2PConfig(map[string]string{
		"destinations": `[{"id":" card ","title":" Сбербанк ","details":" +7 999 000-00-00 ","description":" СБП "}]`,
	})
	if err != nil {
		t.Fatalf("ParseP2PConfig(): %v", err)
	}
	if len(settings.Destinations) != 1 {
		t.Fatalf("destinations length = %d, want 1", len(settings.Destinations))
	}
	destination := settings.Destinations[0]
	if destination.ID != "card" || destination.Title != "Сбербанк" || destination.Details != "+7 999 000-00-00" || destination.Description != "СБП" {
		t.Fatalf("destination was not normalized: %+v", destination)
	}
	if settings.FooterText != DefaultP2PFooterText || settings.SenderLabel != DefaultP2PSenderLabel {
		t.Fatalf("defaults were not applied: %+v", settings)
	}
}

func TestParseP2PConfigRequiresCompleteDestination(t *testing.T) {
	_, err := ParseP2PConfig(map[string]string{"destinations": `[{"id":"card","title":"Сбербанк","details":""}]`})
	if err == nil || !strings.Contains(err.Error(), "заполните заголовок и реквизиты") {
		t.Fatalf("ParseP2PConfig() error = %v", err)
	}
}

func TestParseP2PConfigRejectsDuplicateIDs(t *testing.T) {
	_, err := ParseP2PConfig(map[string]string{"destinations": `[
		{"id":"same","title":"Первый","details":"1"},
		{"id":"same","title":"Второй","details":"2"}
	]`})
	if err == nil || !strings.Contains(err.Error(), "уникальными") {
		t.Fatalf("ParseP2PConfig() error = %v", err)
	}
}
