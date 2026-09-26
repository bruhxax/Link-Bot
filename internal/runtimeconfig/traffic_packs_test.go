package runtimeconfig

import "testing"

func TestValidateTrafficPacks(t *testing.T) {
	packs := []TrafficPackSettings{{ID: "traffic_unlimited", Enabled: true, TrafficGB: 0, PriceRub: 50}}
	if err := validateTrafficPacks(&packs); err != nil {
		t.Fatalf("unlimited traffic pack rejected: %v", err)
	}
	if packs[0].PriceStars <= 0 {
		t.Fatal("Stars price was not derived")
	}
	packs[0].TrafficGB = -1
	if err := validateTrafficPacks(&packs); err == nil {
		t.Fatal("negative traffic amount was accepted")
	}
}
