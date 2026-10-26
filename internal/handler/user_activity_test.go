package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
)

type userActivityMemory struct {
	entries []database.UserActivity
	calls   int
	ready   bool
	err     error
}

func (m *userActivityMemory) FindByTelegramId(_ context.Context, _ int64) (*database.Customer, error) {
	m.calls++
	if !m.ready {
		return nil, nil
	}
	return &database.Customer{ID: 42}, nil
}
func (m *userActivityMemory) RecordUserActivity(_ context.Context, e database.UserActivity) error {
	m.entries = append(m.entries, e)
	return m.err
}

func TestUserActivityTelegramKeepsPrivateDataOutAndRunsHandler(t *testing.T) {
	repo := &userActivityMemory{err: errors.New("database failure")}
	called := 0
	next := UserActivityMiddleware(repo)(func(context.Context, *bot.Bot, *models.Update) { called++; repo.ready = true })
	message := &models.Message{From: &models.User{ID: 9}, Chat: models.Chat{ID: 9, Type: models.ChatTypePrivate}, Text: "/start secret-referral-token"}
	next(context.Background(), nil, &models.Update{Message: message})
	query := &models.CallbackQuery{From: models.User{ID: 9}, Data: "payment?planId=private-plan&token=secret-token", Message: models.MaybeInaccessibleMessage{Message: message}}
	next(context.Background(), nil, &models.Update{CallbackQuery: query})
	if called != 2 || len(repo.entries) != 2 || repo.entries[0].CustomerID != 42 || repo.entries[1].Status != "info" {
		t.Fatalf("handler/history %+v calls %d", repo, called)
	}
	raw, _ := json.Marshal(repo.entries)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "private") {
		t.Fatal("private callback/command arguments copied")
	}
	for _, change := range []func(){
		func() { message.Text = "ordinary private chat text"; query.Data = "unknown?token=secret" },
		func() { message.Chat.Type = models.ChatTypeSupergroup; query.Data = "payment" },
		func() { message.Chat.Type = models.ChatTypePrivate; message.Chat.ID = 100 },
		func() { message.Chat.ID = 9; query.From.IsBot = true; message.From.IsBot = true },
	} {
		change()
		next(context.Background(), nil, &models.Update{Message: message})
		next(context.Background(), nil, &models.Update{CallbackQuery: query})
	}
	if len(repo.entries) != 2 || repo.calls != 2 || called != 10 {
		t.Fatalf("ineligible update recorded %+v calls %d", repo, called)
	}
}
