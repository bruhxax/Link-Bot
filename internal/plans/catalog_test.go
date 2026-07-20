package plans

import "testing"

func TestStarsForRub(t *testing.T) {
	tests := []struct {
		rubles int
		stars  int
	}{
		{rubles: 89, stars: 61},
		{rubles: 170, stars: 116},
		{rubles: 700, stars: 476},
	}

	for _, test := range tests {
		if got := StarsForRub(test.rubles); got != test.stars {
			t.Fatalf("StarsForRub(%d) = %d, want %d", test.rubles, got, test.stars)
		}
	}
}
