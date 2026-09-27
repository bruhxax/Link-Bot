package payment

import (
	"strings"
	"testing"
	"time"

	"link-bot/internal/database"
)

func TestBuildOrderDescription(t *testing.T) {
	gb := int64(1024 * 1024 * 1024)
	traffic := 1000 * gb
	devices := 5
	until := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		months  int
		options CreatePurchaseOptions
		want    []string
	}{
		{"subscription with packs", 3, CreatePurchaseOptions{TrafficLimitBytes: &traffic, DeviceLimitCount: &devices, ExtraDevices: 1, ExtraTrafficBytes: 50 * gb}, []string{"Подписка на 3 месяца", "5 устройств", "трафик 1000 ГБ", "+1 устройство", "+50 ГБ трафика"}},
		{"devices only", 0, CreatePurchaseOptions{PurchaseKind: database.PurchaseKindExtraDevices, ExtraDevices: 1, DeviceExpiresAt: &until}, []string{"+1 устройство", "15.10.2026"}},
		{"traffic only", 0, CreatePurchaseOptions{PurchaseKind: database.PurchaseKindExtraTraffic, ExtraTrafficBytes: 50 * gb}, []string{"+50 ГБ"}},
		{"unlimited traffic", 1, CreatePurchaseOptions{TrafficLimitBytes: new(int64), DeviceLimitCount: &devices}, []string{"безлимитный трафик", "5 устройств"}},
		{"gift", 1, CreatePurchaseOptions{PurchaseKind: database.PurchaseKindGift, TrafficLimitBytes: &traffic, DeviceLimitCount: &devices}, []string{"Подарок:", "1 месяц", "1000 ГБ"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildOrderDescription(tt.months, tt.options)
			for _, part := range tt.want {
				if !strings.Contains(got, part) {
					t.Fatalf("description %q misses %q", got, part)
				}
			}
			if len([]rune(got)) > orderDescriptionLimit {
				t.Fatalf("description too long: %q", got)
			}
		})
	}
}
