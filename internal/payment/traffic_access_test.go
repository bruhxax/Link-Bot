package payment

import "testing"

func TestTrafficLimitWithGrants(t *testing.T) {
	const gb = int64(1024 * 1024 * 1024)
	for _, tc := range []struct {
		name        string
		base, extra int64
		unlimited   bool
		want        int64
	}{
		{"adds paid traffic to plan", 50 * gb, 10 * gb, false, 60 * gb},
		{"keeps paid traffic on renewal", 100 * gb, 10 * gb, false, 110 * gb},
		{"unlimited pack", 50 * gb, 0, true, 0},
		{"unlimited plan", 0, 10 * gb, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := trafficLimitWithGrants(tc.base, tc.extra, tc.unlimited); got != tc.want {
				t.Fatalf("trafficLimitWithGrants() = %d, want %d", got, tc.want)
			}
		})
	}
}
