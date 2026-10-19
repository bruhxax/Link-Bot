package runtimeconfig

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPaymentVariantsPersistIndependentChannelsAndOrder(t *testing.T) {
	s := DefaultSettings()
	s.PaymentMethodVariants = map[string]PaymentMethodVariant{
		"sbp":           {Method: "sbp", Channels: []string{" sbp ", "sberbank", "sbp"}},
		"sbp__aabbccdd": {Method: "sbp", Channels: []string{"bank_card"}},
	}
	s.PaymentMethodText = map[string]PaymentMethodText{"sbp__aabbccdd": {Name: "Карта"}}
	s.PaymentMethodOrder = []string{"sbp__aabbccdd", "sbp"}
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.PaymentMethodVariants["sbp"].Channels, []string{"sbp", "sberbank"}) {
		t.Fatal("duplicate channels not normalized")
	}
	if s.PaymentMethodOrder[0] != "sbp__aabbccdd" {
		t.Fatal("copy order lost")
	}
	raw, _ := json.Marshal(s)
	var loaded Settings
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.PaymentMethodBase("sbp__aabbccdd") != "sbp" || loaded.PaymentMethodText["sbp__aabbccdd"].Name != "Карта" {
		t.Fatal("copy not persisted")
	}
	clone := cloneSettings(s)
	clone.PaymentMethodVariants["sbp"].Channels[0] = "yoo_money"
	if s.PaymentMethodVariants["sbp"].Channels[0] != "sbp" {
		t.Fatal("snapshot shares channel slice")
	}
}

func TestPaymentVariantsRejectForgedProvidersAndChannels(t *testing.T) {
	for id, v := range map[string]PaymentMethodVariant{
		"card__aabbccdd":  {Method: "sbp", Channels: []string{"sbp"}},
		"card__oops":      {Method: "card"},
		"stars__aabbccdd": {Method: "stars"},
		"card":            {Method: "card", Channels: []string{"sbp&amount=1"}},
		"wata":            {Method: "wata", Channels: []string{"card"}},
	} {
		s := DefaultSettings()
		s.PaymentMethodVariants = map[string]PaymentMethodVariant{id: v}
		if NormalizeAndValidate(&s) == nil {
			t.Errorf("accepted %s %+v", id, v)
		}
	}
}
