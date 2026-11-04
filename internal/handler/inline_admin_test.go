package handler

import "testing"

func TestParseInlineAdminCommand(t *testing.T) {
	tests := []struct {
		input, kind, search string
		value               int64
		ok                  bool
	}{
		{"user @alice", "user", "@alice", 0, true},
		{"пользователь 123456", "user", "123456", 0, true},
		{"продлить 30 @alice", "extend", "@alice", 30, true},
		{"balance 150 123456", "balance", "123456", 150, true},
		{"промо AUTUMN", "promo", "AUTUMN", 0, true},
		{"Месяц", "", "", 0, false},
	}
	for _, tc := range tests {
		got, ok := parseInlineAdminCommand(tc.input)
		if ok != tc.ok || got.Kind != tc.kind || got.Search != tc.search || got.Value != tc.value {
			t.Errorf("parseInlineAdminCommand(%q) = %+v, %v", tc.input, got, ok)
		}
	}
}

func TestInlineAdminNumericQuery(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"123456789", true}, {"", false}, {"123abc", false}, {"@alice", false},
	} {
		if got := isInlineAdminNumericQuery(tc.input); got != tc.want {
			t.Errorf("isInlineAdminNumericQuery(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
