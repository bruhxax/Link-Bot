package database

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectMessageDraftHidesTelegramSource(t *testing.T) {
	chatID, messageID := int64(12345), 42
	draft := DirectMessageDraft{
		AdminTelegramID: 1, CustomerID: 2, Status: "draft",
		SourceChatID: &chatID, SourceMessageID: &messageID,
		SourceKind: "html", SourcePreview: "Preview", SourceHTML: "<b>Secret</b>",
	}
	if !draft.HasSource() {
		t.Fatal("valid source should be present")
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"sourceChatId", "sourceMessageId", "sourceHtml", "Secret", "adminTelegramId"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("direct message API leaked %q: %s", secret, raw)
		}
	}
	if !strings.Contains(string(raw), `"customerId":2`) {
		t.Fatalf("recipient missing: %s", raw)
	}
}
