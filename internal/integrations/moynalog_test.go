package integrations

import "testing"

func TestParseMoyNalogConfig(t *testing.T) {
	cfg, err := ParseMoyNalogConfig(map[string]string{
		"username":       "123456789012",
		"password":       "secret",
		"itemName":       "VPN-доступ",
		"paymentMethods": `["yookassa","p2p"]`,
	})
	if err != nil {
		t.Fatalf("ParseMoyNalogConfig: %v", err)
	}
	if cfg.APIURL != DefaultMoyNalogAPIURL || !cfg.PaymentMethods[ProviderYooKassa] || !cfg.PaymentMethods[ProviderP2P] {
		t.Fatalf("unexpected parsed config: %#v", cfg)
	}
}

func TestParseMoyNalogConfigRejectsInvalidINNAndMethod(t *testing.T) {
	for name, raw := range map[string]map[string]string{
		"inn":    {"username": "123", "password": "secret", "paymentMethods": `["yookassa"]`},
		"method": {"username": "123456789012", "password": "secret", "paymentMethods": `["unknown"]`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseMoyNalogConfig(raw); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
