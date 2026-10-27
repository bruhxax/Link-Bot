package broadcast

import (
	"context"
	"errors"
	"testing"
	"time"

	"link-bot/internal/database"
)

func TestSurveySendsWithoutCapturedMessageAndKeepsOrdinaryDraft(t *testing.T) {
	f, ctx := newPinFixture(t)
	repo := database.NewBroadcastRepository(f.pool)
	d, err := f.s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Preview(ctx, 900); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("empty ordinary draft: %v", err)
	}
	ordinary := []database.BroadcastButton{{ID: "ordinary", Type: "main", Text: "Меню"}}
	d, err = repo.SaveButtons(ctx, ordinary, 900)
	if err != nil {
		t.Fatal(err)
	}
	cfg := surveyTestConfig()
	cfg.IntroText = "<b>Помогите нам стать лучше</b>"
	cfg.FinalButtons = []database.BroadcastButton{{ID: "survey", Type: "support", Text: "Поддержка"}}
	d, err = f.s.SaveSurvey(ctx, 900, cfg, d.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.Preview(ctx, 900)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceKind != "" || len(preview.Buttons) != 1 || preview.Buttons[0].ID != "ordinary" {
		t.Fatalf("preview replaced ordinary editor state: %+v", preview)
	}
	f.s.SetAudienceResolver(func(context.Context, database.BroadcastAudience) ([]int64, error) { return []int64{101}, nil })
	started, err := f.s.Start(ctx, 900, d.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if started.SourceKind != "" || started.Buttons[0].ID != "ordinary" {
		t.Fatal("send response replaced ordinary editor state")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		d, err = repo.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if d.Status != database.BroadcastStatusRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("survey did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if d.SentCount != 1 || d.SourceKind != "" || d.Buttons[0].ID != "ordinary" {
		t.Fatalf("ordinary draft changed: %+v", d)
	}
	runs, err := repo.SurveyRuns(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	run, err := repo.SurveyRun(ctx, runs[0].ID)
	if err != nil || len(run.Buttons) != 1 || run.Buttons[0].ID != "survey" {
		t.Fatalf("independent final buttons: %+v %v", run, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call.Method == "sendMessage" && call.ChatID == 101 && call.Text != cfg.IntroText {
			t.Fatalf("wrong invitation: %s", call.Text)
		}
	}
}

func TestStandaloneSurveyTemplateAndExplicitlyEmptyFinalButtons(t *testing.T) {
	f, ctx := newPinFixture(t)
	d, err := f.s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := surveyTestConfig()
	cfg.IntroText = "Опрос без сообщения в боте"
	cfg.FinalButtons = []database.BroadcastButton{}
	p := database.BroadcastTemplatePayload{Survey: &cfg, Buttons: []database.BroadcastButton{{ID: "ad", Type: "main", Text: "Реклама"}}}
	d, err = f.s.RestoreTemplate(ctx, p, 900, d.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.Preview(ctx, 900)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Survey.FinalButtons == nil || len(preview.Survey.FinalButtons) != 0 {
		t.Fatal("empty survey buttons inherited ordinary buttons")
	}
	if !preview.HasSource() {
		t.Fatal("standalone survey cannot be saved as a template")
	}
	cfg.Enabled = false
	if _, err = f.s.SaveSurvey(ctx, 900, cfg, d.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Preview(ctx, 900); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("ordinary draft must require its own message: %v", err)
	}
}

func TestSurveyInvitationValidationAndOutOfOrderCallback(t *testing.T) {
	cfg := surveyTestConfig()
	cfg.IntroText = "<b>Invitation</b>"
	if err := NormalizeSurvey(&cfg); err != nil {
		t.Fatal(err)
	}
	text, _ := surveyScreen(&database.SurveyRun{Config: cfg}, -1)
	if text != cfg.IntroText {
		t.Fatal("out-of-order callback replaced invitation with internal title")
	}
	cfg.IntroText = "<b>broken"
	if NormalizeSurvey(&cfg) == nil {
		t.Fatal("invalid invitation HTML accepted")
	}
}

func TestLegacySurveyKeepsCapturedInvitation(t *testing.T) {
	cfg := surveyTestConfig()
	draft := &database.BroadcastDraft{SourceKind: "html", SourceHTML: "<b>Наше приглашение</b>"}
	hydrateSurveyInvitation(&cfg, draft)
	if err := NormalizeSurvey(&cfg); err != nil {
		t.Fatal(err)
	}
	applySurveyInvitation(draft, &cfg)
	if draft.SourceHTML != "<b>Наше приглашение</b>" {
		t.Fatal("existing invitation was lost")
	}
}
