package database

import "testing"

func TestBroadcastHTMLPreviewRetainsTextWithoutMarkup(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{"<b>Привет</b> &amp; <i>спасибо</i>\nВторая строка", "Привет & спасибо\nВторая строка"},
		{"<tg-emoji emoji-id=\"123456789\">🎉</tg-emoji> <a href=\"https://example.com\">Ссылка</a>", "🎉 Ссылка"},
		{"&lt;script&gt;alert(1)&lt;/script&gt;", "<script>alert(1)</script>"},
	} {
		if got := broadcastPreviewText(test.source); got != test.want {
			t.Errorf("preview %q: got %q, want %q", test.source, got, test.want)
		}
	}
}
