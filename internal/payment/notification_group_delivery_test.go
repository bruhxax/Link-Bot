package payment

import (
	"encoding/json"
	"testing"
)

func TestTelegramNotificationDestinationIncludesTopicOnlyWhenSet(t *testing.T) {
	for _, test := range []struct {
		name     string
		threadID int
		want     bool
	}{
		{name: "ordinary group", threadID: 0},
		{name: "forum topic", threadID: 42, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(telegramSendMessageRequest{ChatID: -100123, MessageThreadID: test.threadID, Text: "Оплата"})
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if _, exists := fields["message_thread_id"]; exists != test.want {
				t.Fatalf("message_thread_id presence = %v, want %v: %s", exists, test.want, raw)
			}
		})
	}
}
