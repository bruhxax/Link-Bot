package paymentchannel

import "testing"

func TestResolveCannotEscapeConfiguredChannels(t *testing.T) {
	for _, tc := range []struct {
		method   string
		allowed  []string
		selected string
		fail     bool
		count    int
	}{
		{"smerch__aabbccdd", []string{"sbp", "crypto"}, "sbp", false, 1},
		{"smerch", []string{"sbp", "crypto"}, "", true, 0},
		{"smerch", []string{"sbp"}, "crypto", true, 0},
		{"smerch", []string{"sbp_recurring"}, "", true, 0},
		{"sbp", []string{"sbp", "sberbank"}, "bank_card", true, 0},
		{"card__aabbccdd", []string{"bank_card"}, "", false, 1},
		{"sbp", []string{"sbp", "sberbank"}, "", true, 0},
		{"sbp", []string{"sbp", "sberbank"}, "sberbank", false, 1},
		{"card", nil, "bank_card", true, 0},
		{"card", nil, "", false, 0},
		{"anore", []string{"sbp", "card"}, "", false, 2},
		{"wata", []string{"card"}, "", true, 0},
		{"heleket", []string{"USDT:tron", "BTC:btc"}, "", false, 2},
		{"heleket", []string{"USDT:<script>"}, "", true, 0},
	} {
		t.Run(tc.method+tc.selected, func(t *testing.T) {
			got, err := Resolve(tc.method, tc.allowed, tc.selected)
			if (err != nil) != tc.fail || len(got) != tc.count {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

func TestEveryGatewayHasExplicitCapabilities(t *testing.T) {
	for _, method := range []string{"sbp", "card", "crypto", "lava", "wata", "platega", "freekassa", "heleket", "pally", "rollypay", "cispay", "anore", "mulenpay", "aurapay", "antilopay", "paritypay", "tribute", "cloudpayments", "datagio", "kassaai", "payhot", "smerch"} {
		c := For(method)
		if c.Note == "" {
			t.Errorf("no capability explanation for %s", method)
		}
		if len(c.Options) == 0 && method != "wata" && method != "mulenpay" && method != "cloudpayments" && method != "datagio" {
			t.Errorf("methods missing for %s", method)
		}
		for _, option := range c.Options {
			if !Valid(method, option.ID) || option.Name == "" {
				t.Errorf("invalid %s option %+v", method, option)
			}
		}
	}
	if !For("tribute").Preference || !For("aurapay").Preference || !For("antilopay").Preference {
		t.Fatal("preferences incorrectly advertised as restrictions")
	}
}
