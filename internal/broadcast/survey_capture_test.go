package broadcast

import (
	"errors"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
	"strings"
	"testing"
	"time"
)

func TestSurveyCaptureEntityFormatting(t *testing.T) {
	cases := []struct {
		name, text string
		entities   []models.MessageEntity
		want       string
	}{
		{"plain escape", "1 < 2 & 3 > 2", nil, "1 &lt; 2 &amp; 3 &gt; 2"},
		{"plain angle letter", "A < B & C", nil, "A &lt; B &amp; C"},
		{"utf16 nested", "😀 Жирный", []models.MessageEntity{{Type: models.MessageEntityTypeBold, Offset: 3, Length: 6}, {Type: models.MessageEntityTypeItalic, Offset: 3, Length: 6}}, "😀 <b><i>Жирный</i></b>"},
		{"adjacent", "abc", []models.MessageEntity{{Type: models.MessageEntityTypeBold, Offset: 0, Length: 1}, {Type: models.MessageEntityTypeItalic, Offset: 1, Length: 2}}, "<b>a</b><i>bc</i>"},
		{"link", "Сайт", []models.MessageEntity{{Type: models.MessageEntityTypeTextLink, Length: 4, URL: "https://example.com/?a=1&b=2"}}, `<a href="https://example.com/?a=1&amp;b=2">Сайт</a>`},
		{"emoji", "🔥", []models.MessageEntity{{Type: models.MessageEntityTypeCustomEmoji, Length: 2, CustomEmojiID: "5206222720416643915"}}, `<tg-emoji emoji-id="5206222720416643915">🔥</tg-emoji>`},
		{"quote", "Цитата", []models.MessageEntity{{Type: models.MessageEntityTypeExpandableBlockquote, Length: 6}}, "<blockquote expandable>Цитата</blockquote>"},
		{"code", "x < y", []models.MessageEntity{{Type: models.MessageEntityTypePre, Length: 5, Language: "go"}}, `<pre><code class="language-go">x &lt; y</code></pre>`},
		{"spoiler", "Тайна", []models.MessageEntity{{Type: models.MessageEntityTypeSpoiler, Length: 5}}, "<tg-spoiler>Тайна</tg-spoiler>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := surveyMessageHTML(tc.text, tc.entities)
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v, want %q", got, err, tc.want)
			}
			if surveyHTML(got, true) != got {
				t.Fatal("formatted text double escaped")
			}
		})
	}
	for _, tc := range []struct {
		text string
		e    []models.MessageEntity
	}{
		{"😀x", []models.MessageEntity{{Type: models.MessageEntityTypeBold, Offset: 1, Length: 1}}},
		{"abcd", []models.MessageEntity{{Type: models.MessageEntityTypeBold, Length: 3}, {Type: models.MessageEntityTypeItalic, Offset: 2, Length: 2}}},
		{"x", []models.MessageEntity{{Type: models.MessageEntityTypeTextLink, Length: 1, URL: "javascript:alert(1)"}}},
		{"x", []models.MessageEntity{{Type: models.MessageEntityTypeCustomEmoji, Length: 1, CustomEmojiID: "invalid"}}},
		{strings.Repeat("😀", 1751), nil},
	} {
		if _, err := surveyMessageHTML(tc.text, tc.e); err == nil {
			t.Fatalf("invalid formatting accepted: %q", tc.text[:1])
		}
	}
	if err := validateSurveyMessage("<b>"+strings.Repeat("x", 3500)+"</b>", true); err != nil {
		t.Fatal(err)
	}
}
func TestSurveyDraftAllowsIncompleteButDeliveryRejectsIt(t *testing.T) {
	cfg := database.SurveyConfig{Enabled: true, Questions: []database.SurveyQuestion{{Options: []string{""}}}}
	if err := NormalizeSurveyDraft(&cfg); err != nil {
		t.Fatal(err)
	}
	if err := NormalizeSurvey(&cfg); err == nil {
		t.Fatal("incomplete survey can be delivered")
	}
}
func TestSurveyCapturePersistsFormatsAndDoesNotReplaceOrdinaryMessage(t *testing.T) {
	f, ctx := newPinFixture(t)
	draft, err := f.s.repository.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := surveyTestConfig()
	cfg.Questions[1].Text = ""
	draft, err = f.s.SaveSurveyDraft(ctx, 900, cfg, draft.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	originalKind, originalText := draft.SourceKind, draft.SourceText
	draft, err = f.s.StartSurveyCapture(ctx, 900, "question", 1, cfg, draft.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.loadSurvey(ctx, draft); !errors.Is(err, ErrSurveyCapturePending) {
		t.Fatalf("preview while capturing: %v", err)
	}
	id := draft.Survey.Capture.ID
	if _, err = f.s.StartCapture(ctx, 900); !errors.Is(err, ErrSurveyCapturePending) {
		t.Fatalf("ordinary capture not blocked: %v", err)
	}
	if _, err = f.s.SaveSurveyDraft(ctx, 900, cfg, draft.UpdatedAt); !errors.Is(err, ErrSurveyCapturePending) {
		t.Fatalf("survey overwrote pending input: %v", err)
	}
	// A second service instance simulates restart: the capture context lives in PostgreSQL.
	restarted := NewService(database.NewBroadcastRepository(f.pool), nil, nil, f.s.telegramBot)
	message := &models.Message{ID: 55, From: &models.User{ID: 901}, Chat: models.Chat{ID: 901, Type: models.ChatTypePrivate}, Text: "Wrong admin"}
	if ok, err := restarted.CaptureMessage(ctx, message); ok || err != nil {
		t.Fatalf("another admin consumed: %v %v", ok, err)
	}
	message.From.ID = 900
	message.Chat.ID = 900
	message.Text = "/start"
	if ok, err := restarted.CaptureMessage(ctx, message); ok || err != nil {
		t.Fatalf("command consumed: %v %v", ok, err)
	}
	message.Text = ""
	message.Caption = "Photo caption"
	if ok, err := restarted.CaptureMessage(ctx, message); !ok || err != nil {
		t.Fatalf("unsupported message: %v %v", ok, err)
	}
	pending, _ := restarted.Get(ctx)
	if pending.Survey.Capture.ID != id {
		t.Fatal("invalid input dropped capture")
	}
	message.Text = "😀 Вопрос & ответ"
	message.Entities = []models.MessageEntity{{Type: models.MessageEntityTypeBold, Offset: 3, Length: 6}}
	if ok, err := restarted.CaptureMessage(ctx, message); !ok || err != nil {
		t.Fatalf("capture %v %v", ok, err)
	}
	saved, err := restarted.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	q := saved.Survey.Questions[1]
	if q.Text != "😀 <b>Вопрос</b> &amp; ответ" || !q.TextHTML || saved.Survey.Capture != nil {
		t.Fatalf("format lost: %+v", q)
	}
	if saved.SourceKind != originalKind || saved.SourceText != originalText {
		t.Fatal("ordinary broadcast replaced")
	}
	// Cancel keeps the previous message and stale tabs cannot cancel a newer capture.
	pending, err = restarted.StartSurveyCapture(ctx, 900, "question", 1, *saved.Survey, saved.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.CancelSurveyCapture(ctx, 900, id); !errors.Is(err, ErrDraftChanged) {
		t.Fatalf("stale cancellation %v", err)
	}
	message.Text = "/cancel"
	message.Entities = nil
	if ok, err := restarted.CaptureMessage(ctx, message); !ok || err != nil {
		t.Fatalf("cancel %v %v", ok, err)
	}
	saved, _ = restarted.Get(ctx)
	if saved.Survey.Capture != nil || saved.Survey.Questions[1].Text != q.Text {
		t.Fatal("cancel lost previous question")
	}
	if pending.Survey.Capture == nil || time.Since(pending.Survey.Capture.StartedAt) > time.Minute {
		t.Fatal("missing capture timestamp")
	}
	for _, target := range []string{"intro", "finish"} {
		pending, err = restarted.StartSurveyCapture(ctx, 900, target, 0, *saved.Survey, saved.UpdatedAt)
		if err != nil {
			t.Fatal(err)
		}
		message.Text = "A < B & C"
		message.Entities = nil
		if ok, err := restarted.CaptureMessage(ctx, message); !ok || err != nil {
			t.Fatalf("%s capture %v %v", target, ok, err)
		}
		saved, err = restarted.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if target == "intro" && (!saved.Survey.IntroHTML || saved.Survey.IntroText != "A &lt; B &amp; C") {
			t.Fatalf("intro format lost: %+v", saved.Survey)
		}
		if target == "finish" && (!saved.Survey.FinishHTML || saved.Survey.FinishText != "A &lt; B &amp; C") {
			t.Fatal("finish format lost")
		}
	}

}
