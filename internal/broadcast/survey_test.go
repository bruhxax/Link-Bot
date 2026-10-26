package broadcast

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
)

func surveyTestConfig() database.SurveyConfig {
	return database.SurveyConfig{Enabled: true, Title: "Обратная связь", StartText: "Пройти", Questions: []database.SurveyQuestion{{Text: "<b>Как сервис?</b>", Options: []string{"Хорошо", "Плохо"}}, {Text: "Что улучшить?", Options: []string{"Скорость", "Приложение"}}}, FinishText: "Спасибо!"}
}
func TestSurveyValidationAndCallbackSize(t *testing.T) {
	cfg := surveyTestConfig()
	if err := NormalizeSurvey(&cfg); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*database.SurveyConfig){func(c *database.SurveyConfig) { c.Title = "" }, func(c *database.SurveyConfig) { c.StartText = strings.Repeat("a", 65) }, func(c *database.SurveyConfig) { c.Questions = nil }, func(c *database.SurveyConfig) { c.Questions[0].Text = "<b>broken" }, func(c *database.SurveyConfig) { c.Questions[0].Options = []string{} }, func(c *database.SurveyConfig) { c.FinishText = "" }} {
		c := surveyTestConfig()
		change(&c)
		if NormalizeSurvey(&c) == nil {
			t.Fatal("invalid survey accepted")
		}
	}
	id := "f3de18b3-10c7-4599-a067-fc9ef04957d1"
	if len(surveyData(id, 63, 99)) > 64 {
		t.Fatal("callback exceeds Telegram limit")
	}
	run := &database.SurveyRun{ID: id, Config: cfg, Buttons: []database.BroadcastButton{{Type: "main", Text: "Главная"}}}
	text, keys := surveyScreen(run, 0)
	if !strings.Contains(text, "1 / 2") || len(keys.(*models.InlineKeyboardMarkup).InlineKeyboard) != 2 {
		t.Fatal("question screen invalid")
	}
	text, keys = surveyScreen(run, 2)
	if text != "Спасибо!" || len(keys.(*models.InlineKeyboardMarkup).InlineKeyboard) != 1 {
		t.Fatal("final buttons lost")
	}
}

func TestSurveyEditsOneMessageAndPersistsResponses(t *testing.T) {
	f, ctx := newPinFixture(t)
	if _, err := f.pool.Exec(ctx, "ALTER TABLE customer ADD COLUMN username TEXT; UPDATE customer SET username='fixture_user' WHERE telegram_id=101"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var calls []string
	var messages []int
	failEdit := false
	lastText := ""
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = r.ParseMultipartForm(1 << 20)
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		calls = append(calls, method)
		message, _ := strconv.Atoi(r.FormValue("message_id"))
		if method == "editMessageText" {
			messages = append(messages, message)
			lastText = r.FormValue("text")
			if failEdit {
				failEdit = false
				w.WriteHeader(500)
				fmt.Fprint(w, `{"ok":false,"error_code":500,"description":"temporary error"}`)
				return
			}
		}
		if method == "answerCallbackQuery" || method == "deleteMessage" {
			fmt.Fprint(w, `{"ok":true,"result":true}`)
			return
		}
		if message == 0 {
			message = 501
		}
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"chat":{"id":101,"type":"private"}}}`, message)
	}))
	defer api.Close()
	b, err := bot.New("123:fixture", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	f.s.telegramBot = b
	repo := database.NewBroadcastRepository(f.pool)
	d, err := repo.StartCapture(ctx, 900)
	if err != nil {
		t.Fatal(err)
	}
	d, err = repo.SaveSource(ctx, 900, 10, "html", "Пройдите опрос", "Пройдите опрос", 900)
	if err != nil {
		t.Fatal(err)
	}
	cfg := surveyTestConfig()
	d, err = f.s.SaveSurvey(ctx, 900, cfg, d.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	oldStamp := d.UpdatedAt
	if _, err = f.s.SaveSurvey(ctx, 900, cfg, oldStamp.Add(-1)); err != ErrDraftChanged {
		t.Fatal("stale settings overwrote draft")
	}
	id, err := repo.CreateSurveyRun(ctx, cfg, []database.BroadcastButton{{Type: "main", Text: "Меню"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	d.SurveyRunID = id
	if err = f.s.deliverBroadcast(ctx, *d, 101, surveyIntroKeyboard(id, &cfg)); err != nil {
		t.Fatal(err)
	}
	click := func(chat int64, message, step, option int) {
		f.s.HandleSurvey(ctx, b, &models.Update{CallbackQuery: &models.CallbackQuery{ID: "callback", From: models.User{ID: chat}, Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: message, Chat: models.Chat{ID: chat, Type: models.ChatTypePrivate}}}, Data: surveyData(id, step, option)}})
	}
	click(102, 501, -1, -1) // Another user cannot answer someone else's invitation.
	click(101, 999, -1, -1)
	click(101, 501, 0, 1) // A future callback must restore the start button without recording a vote.
	click(101, 501, -1, -1)
	mu.Lock()
	failEdit = true
	mu.Unlock()
	click(101, 501, 0, 1) // Telegram failure rolls the transition back; retry is safe.
	report, err := repo.SurveyReport(ctx, id, 0)
	if err != nil || report.Run.Started != 1 || len(report.Recipients[0].Answers) != 0 {
		t.Fatalf("failed edit persisted answer: %+v %v", report, err)
	}
	click(101, 501, 0, 1)
	click(101, 501, 0, 0) // Double click must not change the saved answer.
	// A restarted service reads the immutable run, even after editing the draft.
	cfg.Questions[0].Options = []string{"Changed"}
	cfg.FinishText = "Changed"
	if _, err = f.s.SaveSurvey(ctx, 900, cfg, oldStamp); err != nil {
		t.Fatal(err)
	}
	f.s = NewService(repo, nil, nil, b)
	click(101, 501, 1, 0)
	click(101, 501, 1, 1)
	report, err = repo.SurveyReport(ctx, id, 0)
	if err != nil || report.Run.Delivered != 1 || report.Run.Completed != 1 || report.Counts[0][1] != 1 || report.Counts[1][0] != 1 || report.Recipients[0].Username != "fixture_user" {
		t.Fatalf("report %+v %v", report, err)
	}
	if got := report.Recipients[0].Answers; len(got) != 2 || got[0] != 1 || got[1] != 0 {
		t.Fatal("duplicate or changed answers", got)
	}

	previewID, err := repo.CreateSurveyRun(ctx, surveyTestConfig(), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.RegisterSurveyRecipient(ctx, previewID, 900, 502); err != nil {
		t.Fatal(err)
	}
	runs, err := repo.SurveyRuns(ctx)
	if err != nil || len(runs) != 1 || runs[0].ID != id {
		t.Fatalf("previews leaked into results: %+v %v", runs, err)
	}
	if previewReport, err := repo.SurveyReport(ctx, previewID, 0); err != nil || previewReport != nil {
		t.Fatal("preview visible in reports")
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO broadcast_survey_session(run_id,telegram_id,message_id) SELECT $1,10000+n,700+n FROM generate_series(1,60) n`, id); err != nil {
		t.Fatal(err)
	}
	first, err := repo.SurveyReport(ctx, id, 0)
	if err != nil || len(first.Recipients) != 50 || !first.HasMore || first.Run.Delivered != 61 {
		t.Fatalf("first page %+v %v", first, err)
	}
	second, err := repo.SurveyReport(ctx, id, first.Recipients[49].ID)
	if err != nil || len(second.Recipients) != 11 || second.HasMore {
		t.Fatalf("next page %+v %v", second, err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, message := range messages {
		if message != 501 {
			t.Fatal("edited a different message")
		}
	}
	sent := 0
	for _, method := range calls {
		if method == "sendMessage" {
			sent++
		}
	}
	if sent != 1 || lastText != "Спасибо!" {
		t.Fatalf("sent=%d last=%s", sent, lastText)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "Changed") {
		t.Fatal("draft edits changed sent survey")
	}
}
