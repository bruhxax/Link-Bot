package runtimeconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPaymentMethodTextNormalizationAndPersistence(t *testing.T) {
	s := DefaultSettings()
	s.PaymentMethodText = map[string]PaymentMethodText{
		"sbp":   {Name: "  СБП без комиссии  ", Description: "  Моментально  "},
		"card":  {Name: " ", Description: ""},
		"stars": {Name: "", Description: "  Оплата звёздами  "},
	}
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.PaymentMethodText["card"]; ok {
		t.Fatal("empty override should restore system values")
	}
	if s.PaymentMethodText["sbp"].Name != "СБП без комиссии" || s.PaymentMethodText["stars"].Name != "" || s.PaymentMethodText["stars"].Description != "Оплата звёздами" {
		t.Fatal("fields were not independently normalized")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Settings
	if err = json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.PaymentMethodText["sbp"] != s.PaymentMethodText["sbp"] {
		t.Fatal("custom payment text lost on round trip")
	}
	cloned := cloneSettings(s)
	cloned.PaymentMethodText["sbp"] = PaymentMethodText{Name: "Other"}
	if s.PaymentMethodText["sbp"].Name == "Other" {
		t.Fatal("snapshot shares editable map")
	}
}

func TestPaymentMethodTextRejectsUnknownMethodsAndOversizedFields(t *testing.T) {
	for _, custom := range []map[string]PaymentMethodText{
		{"unknown": {Name: "Test"}},
		{"card": {Name: strings.Repeat("Я", 81)}},
		{"card": {Description: strings.Repeat("Я", 241)}},
	} {
		s := DefaultSettings()
		s.PaymentMethodText = custom
		if NormalizeAndValidate(&s) == nil {
			t.Fatal("invalid custom payment text accepted")
		}
	}
	s := DefaultSettings()
	s.PaymentMethodText = map[string]PaymentMethodText{"card": {Name: strings.Repeat("😊", 80)}}
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal("limit must count characters, not UTF-8 bytes", err)
	}
}
