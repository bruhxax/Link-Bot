package integrations

import (
	"encoding/json"
	"testing"
)

func TestParseNotificationGroups(t *testing.T) {
	want := []NotificationGroup{
		{ChatID: -100123456, Title: "Обычная", IsForum: false},
		{ChatID: -100789012, Title: "Форум", IsForum: true, ThreadID: 42},
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got := ParseNotificationGroups(map[string]string{"groups": string(raw)})
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("parsed destinations = %#v, want %#v", got, want)
	}
	if groups := ParseNotificationGroups(map[string]string{"groups": "invalid"}); len(groups) != 0 {
		t.Fatalf("malformed group config should not produce destinations: %#v", groups)
	}
}
