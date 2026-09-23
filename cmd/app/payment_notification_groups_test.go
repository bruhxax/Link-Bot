package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"link-bot/internal/integrations"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type groupSettingsStub struct {
	groups map[int64]integrations.NotificationGroup
}

func (s *groupSettingsStub) StoredConfig(string) (map[string]string, bool) {
	groups := make([]integrations.NotificationGroup, 0, len(s.groups))
	for _, group := range s.groups {
		groups = append(groups, group)
	}
	raw, _ := json.Marshal(groups)
	return map[string]string{"groups": string(raw)}, true
}

func (s *groupSettingsStub) SaveNotificationGroup(_ context.Context, group integrations.NotificationGroup) error {
	s.groups[group.ChatID] = group
	return nil
}

func (s *groupSettingsStub) RemoveNotificationGroup(_ context.Context, chatID int64) error {
	delete(s.groups, chatID)
	return nil
}

type sentTelegramMessage struct {
	ChatID          int64  `json:"chat_id"`
	MessageThreadID int    `json:"message_thread_id"`
	Text            string `json:"text"`
	ReplyMarkup     string `json:"reply_markup"`
}

func newNotificationTestBot(t *testing.T) (*bot.Bot, *[]sentTelegramMessage) {
	t.Helper()
	var mu sync.Mutex
	messages := []sentTelegramMessage{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse sendMessage: %v", err)
			}
			chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
			threadID, _ := strconv.Atoi(r.FormValue("message_thread_id"))
			message := sentTelegramMessage{ChatID: chatID, MessageThreadID: threadID, Text: r.FormValue("text"), ReplyMarkup: r.FormValue("reply_markup")}
			mu.Lock()
			messages = append(messages, message)
			mu.Unlock()
			if message.ReplyMarkup == "null" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: object expected as reply markup"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":-10042,"type":"supergroup"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":5,"is_bot":true,"first_name":"Notifier","username":"HookBruh_bot"}}`))
	}))
	t.Cleanup(server.Close)
	b, err := bot.New("test-token", bot.WithServerURL(server.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	return b, &messages
}

func TestNotificationBotPrivateStartShowsAddToGroupButton(t *testing.T) {
	settings := &groupSettingsStub{groups: map[int64]integrations.NotificationGroup{}}
	b, messages := newNotificationTestBot(t)
	manager := &paymentNotificationGroupManager{settings: settings, adminID: 11}
	manager.handle(context.Background(), b, &models.Update{Message: &models.Message{
		Chat: models.Chat{ID: 11, Type: models.ChatTypePrivate}, From: &models.User{ID: 11}, Text: "/start",
	}})
	if len(*messages) != 1 || !strings.Contains((*messages)[0].ReplyMarkup, "https://t.me/HookBruh_bot?startgroup=payments") {
		t.Fatalf("private /start did not show add-to-group button: %+v", *messages)
	}
}

func TestNotificationGroupCommandsConnectAndSelectTopic(t *testing.T) {
	settings := &groupSettingsStub{groups: map[int64]integrations.NotificationGroup{}}
	b, messages := newNotificationTestBot(t)
	manager := &paymentNotificationGroupManager{settings: settings, adminID: 11, username: "HookBruh_bot"}
	chat := models.Chat{ID: -10042, Type: models.ChatTypeSupergroup, Title: "Test", IsForum: true}
	manager.handle(context.Background(), b, &models.Update{Message: &models.Message{
		Chat: chat, From: &models.User{ID: 11}, Text: "/start@HookBruh_bot payments",
	}})
	if got := settings.groups[chat.ID]; !got.IsForum || got.ThreadID != 0 {
		t.Fatalf("group after /start = %+v, want pending forum", got)
	}
	manager.handle(context.Background(), b, &models.Update{Message: &models.Message{
		Chat: chat, From: &models.User{ID: 11}, Text: "/topic@HookBruh_bot", MessageThreadID: 123,
	}})
	if got := settings.groups[chat.ID].ThreadID; got != 123 {
		t.Fatalf("topic after /topic = %d, want 123", got)
	}
	found := false
	for _, message := range *messages {
		if message.ChatID == chat.ID && message.MessageThreadID == 123 && strings.Contains(message.Text, "Топик подключён") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no confirmation in topic: %+v", *messages)
	}
	manager.handle(context.Background(), b, &models.Update{Message: &models.Message{
		Chat: chat, From: &models.User{ID: 11}, Text: "/start@HookBruh_bot",
	}})
	if got := settings.groups[chat.ID].ThreadID; got != 123 {
		t.Fatalf("repeated /start reset topic to %d", got)
	}
}

func TestNotificationGroupPromotionAndUnauthorizedUser(t *testing.T) {
	settings := &groupSettingsStub{groups: map[int64]integrations.NotificationGroup{}}
	b, messages := newNotificationTestBot(t)
	manager := &paymentNotificationGroupManager{settings: settings, adminID: 11, chatID: 22}
	chat := models.Chat{ID: -10042, Type: models.ChatTypeSupergroup, Title: "Test"}
	manager.handle(context.Background(), b, &models.Update{Message: &models.Message{
		Chat: chat, From: &models.User{ID: 33}, Text: "/connectgroup",
	}})
	if len(settings.groups) != 0 || len(*messages) != 1 {
		t.Fatalf("unauthorized command: groups=%+v messages=%+v", settings.groups, *messages)
	}
	manager.handle(context.Background(), b, &models.Update{MyChatMember: &models.ChatMemberUpdated{
		Chat: chat, From: models.User{ID: 22},
		OldChatMember: models.ChatMember{Type: models.ChatMemberTypeMember},
		NewChatMember: models.ChatMember{Type: models.ChatMemberTypeAdministrator},
	}})
	if got, ok := settings.groups[chat.ID]; !ok || got.IsForum {
		t.Fatalf("promotion did not connect ordinary group: %+v", settings.groups)
	}
}

func TestNotificationGroupIgnoresCommandsForOtherBots(t *testing.T) {
	settings := &groupSettingsStub{groups: map[int64]integrations.NotificationGroup{}}
	b, messages := newNotificationTestBot(t)
	manager := &paymentNotificationGroupManager{settings: settings, adminID: 11, username: "HookBruh_bot"}
	manager.handle(context.Background(), b, &models.Update{Message: &models.Message{
		Chat: models.Chat{ID: -10042, Type: models.ChatTypeSupergroup},
		From: &models.User{ID: 11}, Text: "/start@another_bot",
	}})
	if len(settings.groups) != 0 || len(*messages) != 0 {
		t.Fatalf("command for another bot was handled: groups=%+v messages=%+v", settings.groups, *messages)
	}
}
