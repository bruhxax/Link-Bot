package supportai

import (
	"strings"
	"testing"
)

func TestKnowledgeRedaction(t *testing.T) {
	text := "Иван @ivanuser ivan@example.com https://vpn.example/sub/private vless://secret@server sk_live_private12345 192.168.1.2 +7 (999) 123-45-67 пароль: secret123"
	redacted := Redact(text, "Иван")
	for _, secret := range []string{"Иван", "@ivanuser", "ivan@example.com", "private", "vless://", "sk_live", "192.168", "999", "secret123"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("leaked %q: %s", secret, redacted)
		}
	}
	if got := Redact("Happ на Windows не импортирует подписку"); got != "Happ на Windows не импортирует подписку" {
		t.Fatal("technical evidence corrupted")
	}
}

func TestKnowledgeSearchUnderstandsClientAndPlatform(t *testing.T) {
	query := SearchQuery("Здравствуйте, на пк подписка не добавляется в Нарр https://private.example/secret")
	for _, word := range []string{"нарр", "happ", "windows", "добавляется"} {
		if !strings.Contains(query, word) {
			t.Errorf("missing %s from %s", word, query)
		}
	}
	if strings.Contains(query, "secret") || strings.Contains(query, "здравствуйте") {
		t.Fatal("search includes secrets/noise")
	}
}

func TestShortRepliesOmitAcknowledgementAndQuestionRestatement(t *testing.T) {
	for _, text := range []string{"Понял! Скопируйте ссылку на ПК.", "А, понял. Скопируйте ссылку на ПК.", "Понял: речь о переносе подписки. Скопируйте ссылку на ПК."} {
		if got := CleanReply(text); got != "Скопируйте ссылку на ПК." {
			t.Fatalf("clean reply: %s", got)
		}
	}
	if got := CleanReply("Скопируйте ссылку на ПК."); got != "Скопируйте ссылку на ПК." {
		t.Fatal("useful answer changed")
	}
}
