package integrations

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestLaveshkaSettingsRequireCredentialsAndHideSecrets(t *testing.T) {
	cfg := map[string]string{"apiKey": "LAVE-12-test_fixture", "webhookSecret": "whsec_test_fixture", "rubPerUsd": "90,50"}
	if err := normalizeLaveshkaConfig(cfg); err != nil || cfg["rubPerUsd"] != "90.500000" {
		t.Fatalf("normalization %v %v", cfg, err)
	}
	if !configured(ProviderLaveshka, cfg) || !slices.Contains(SortedPaymentProviders(), ProviderLaveshka) {
		t.Fatal("provider not available")
	}
	for _, key := range []string{"apiKey", "webhookSecret", "rubPerUsd"} {
		copy := map[string]string{}
		for k, v := range cfg {
			if k != key {
				copy[k] = v
			}
		}
		if configured(ProviderLaveshka, copy) {
			t.Errorf("missing %s accepted", key)
		}
	}
	s := &Service{baseURL: "https://vpn.example", records: map[string]record{ProviderLaveshka: {Enabled: true, WebhookToken: "route", Config: cfg}}}
	found := false
	for _, view := range s.ListAdmin() {
		if view.ID != ProviderLaveshka {
			continue
		}
		found = true
		raw, _ := json.Marshal(view)
		if !view.Configured || strings.Contains(string(raw), cfg["apiKey"]) || strings.Contains(string(raw), cfg["webhookSecret"]) {
			t.Fatalf("secrets leaked or missing config: %s", raw)
		}
		if !strings.Contains(view.WebhookURL, "/laveshka/route") {
			t.Fatal("missing webhook route")
		}
		if len(view.Fields) != 3 {
			t.Fatal("unexpected payment method controls")
		}
	}
	if !found {
		t.Fatal("integration missing from admin")
	}
	for _, key := range []string{"bad-key", "LAVE-0-secret", "LAVE-9999999999999999999999-secret", "LAVE-12-secret\nheader"} {
		if err := normalizeLaveshkaConfig(map[string]string{"apiKey": key}); err == nil {
			t.Errorf("invalid key accepted %q", key)
		}
	}
	if err := normalizeLaveshkaConfig(map[string]string{}); err != nil {
		t.Fatal("cannot save disabled empty integration")
	}
}
