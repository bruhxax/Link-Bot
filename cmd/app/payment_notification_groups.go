package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"link-bot/internal/integrations"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type notificationGroupSettings interface {
	StoredConfig(provider string) (map[string]string, bool)
	SaveNotificationGroup(context.Context, integrations.NotificationGroup) error
	RemoveNotificationGroup(context.Context, int64) error
}

type paymentNotificationGroupManager struct {
	settings notificationGroupSettings
	adminID  int64
	chatID   int64
	username string
}

func (m *paymentNotificationGroupManager) isOwner(userID int64) bool {
	return userID != 0 && (userID == m.adminID || (m.chatID > 0 && userID == m.chatID))
}

func (m *paymentNotificationGroupManager) handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil {
		return
	}
	if update.MyChatMember != nil {
		m.handleMembership(ctx, b, update.MyChatMember)
		return
	}
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	command := strings.Fields(strings.TrimSpace(message.Text))
	if message.Chat.Type != models.ChatTypePrivate {
		if len(command) == 0 {
			return
		}
		switch m.commandName(command[0]) {
		case "/start", "/connectgroup", "/topic":
			if !m.isOwner(message.From.ID) {
				m.send(ctx, b, message.Chat.ID, message.MessageThreadID, "Подключить группу может владелец бота уведомлений. Попросите его отправить /connectgroup здесь или /start боту в личном чате.", nil)
				return
			}
			if m.commandName(command[0]) == "/topic" {
				if !message.Chat.IsForum || message.MessageThreadID <= 0 {
					m.send(ctx, b, message.Chat.ID, message.MessageThreadID, "Отправьте /topic внутри нужного топика группы.", nil)
					return
				}
				if _, ok := m.findGroup(message.Chat.ID); !ok {
					m.connectGroup(ctx, b, message.Chat)
				}
				m.configureTopic(ctx, b, message.Chat.ID, message.MessageThreadID, message.Chat.ID, message.MessageThreadID)
			} else {
				m.connectGroup(ctx, b, message.Chat)
			}
		}
		return
	}
	if !m.isOwner(message.From.ID) {
		if len(command) > 0 && strings.HasPrefix(m.commandName(command[0]), "/") {
			m.send(ctx, b, message.Chat.ID, 0, "Настройка уведомлений доступна владельцу бота.", nil)
		}
		return
	}
	if len(command) == 0 {
		return
	}
	switch m.commandName(command[0]) {
	case "/start", "/groups":
		m.sendMenu(ctx, b, message.Chat.ID)
	case "/topic":
		if len(command) != 3 {
			m.send(ctx, b, message.Chat.ID, 0, "Формат: /topic ID_группы ID_топика. ID группы есть в /groups.", nil)
			return
		}
		chatID, chatErr := strconv.ParseInt(command[1], 10, 64)
		threadID, threadErr := strconv.Atoi(command[2])
		if chatErr != nil || threadErr != nil || threadID <= 0 {
			m.send(ctx, b, message.Chat.ID, 0, "Нужны числовые ID группы и топика.", nil)
			return
		}
		m.configureTopic(ctx, b, chatID, threadID, message.Chat.ID, 0)
	case "/remove":
		if len(command) != 2 {
			m.send(ctx, b, message.Chat.ID, 0, "Формат: /remove ID_группы. ID группы есть в /groups.", nil)
			return
		}
		chatID, err := strconv.ParseInt(command[1], 10, 64)
		if err != nil {
			m.send(ctx, b, message.Chat.ID, 0, "Некорректный ID группы.", nil)
			return
		}
		if err := m.settings.RemoveNotificationGroup(ctx, chatID); err != nil {
			slog.Error("notification group removal failed", "error", err, "chat_id", chatID)
			m.send(ctx, b, message.Chat.ID, 0, "Не удалось отключить группу. Попробуйте снова.", nil)
			return
		}
		m.sendMenu(ctx, b, message.Chat.ID)
	default:
		// A single number is convenient immediately after adding one forum group.
		if len(command) == 1 {
			if threadID, err := strconv.Atoi(command[0]); err == nil && threadID > 0 {
				pending := m.pendingForumGroups()
				if len(pending) == 1 {
					m.configureTopic(ctx, b, pending[0].ChatID, threadID, message.Chat.ID, 0)
					return
				}
			}
		}
	}
}

func (m *paymentNotificationGroupManager) commandName(raw string) string {
	name, mentionedBot, mentioned := strings.Cut(raw, "@")
	if mentioned && m.username != "" && !strings.EqualFold(mentionedBot, m.username) {
		return ""
	}
	return name
}

func (m *paymentNotificationGroupManager) handleMembership(ctx context.Context, b *bot.Bot, change *models.ChatMemberUpdated) {
	if change.Chat.Type != models.ChatTypeGroup && change.Chat.Type != models.ChatTypeSupergroup {
		return
	}
	active := func(member models.ChatMember) bool {
		return member.Type == models.ChatMemberTypeMember || member.Type == models.ChatMemberTypeAdministrator || member.Type == models.ChatMemberTypeOwner ||
			(member.Type == models.ChatMemberTypeRestricted && member.Restricted != nil && member.Restricted.IsMember)
	}
	if !active(change.NewChatMember) {
		if err := m.settings.RemoveNotificationGroup(ctx, change.Chat.ID); err != nil {
			slog.Error("notification group removal after bot exit failed", "error", err, "chat_id", change.Chat.ID)
		}
		return
	}
	if !m.isOwner(change.From.ID) {
		m.send(ctx, b, change.Chat.ID, 0, "Бот добавлен. Владелец бота уведомлений должен отправить /connectgroup в этой группе, чтобы включить уведомления об оплате.", nil)
		return
	}
	if _, ok := m.findGroup(change.Chat.ID); ok {
		return
	}
	m.connectGroup(ctx, b, change.Chat)
}

func (m *paymentNotificationGroupManager) connectGroup(ctx context.Context, b *bot.Bot, chat models.Chat) {
	if chat.Type != models.ChatTypeGroup && chat.Type != models.ChatTypeSupergroup {
		return
	}
	group := integrations.NotificationGroup{ChatID: chat.ID, Title: chat.Title, IsForum: chat.IsForum}
	if existing, ok := m.findGroup(chat.ID); ok && existing.IsForum == chat.IsForum {
		group.ThreadID = existing.ThreadID
	}
	if chat.IsForum {
		if group.ThreadID > 0 {
			m.send(ctx, b, chat.ID, 0, fmt.Sprintf("Группа уже подключена. Уведомления приходят в топик %d. Чтобы сменить топик, отправьте /topic в нужном топике.", group.ThreadID), nil)
			return
		}
		if err := m.settings.SaveNotificationGroup(ctx, group); err != nil {
			slog.Error("notification forum registration failed", "error", err, "chat_id", chat.ID)
			m.send(ctx, b, chat.ID, 0, "Не удалось подключить группу. Попробуйте /connectgroup ещё раз.", nil)
			return
		}
		m.send(ctx, b, chat.ID, 0, "Бот добавлен. Для уведомлений в топик администратор должен прислать мне ID топика в личном чате или отправить /topic внутри нужного топика.", nil)
		m.send(ctx, b, m.adminID, 0, fmt.Sprintf("Группа «%s» добавлена. Укажите ID топика для оплат: /topic %d ID_топика. Можно также отправить /topic прямо в нужном топике группы.", chat.Title, chat.ID), nil)
		return
	}
	if !m.send(ctx, b, chat.ID, 0, "Группа подключена: уведомления об оплате будут приходить сюда.", nil) {
		return
	}
	if err := m.settings.SaveNotificationGroup(ctx, group); err != nil {
		slog.Error("notification group registration failed", "error", err, "chat_id", chat.ID)
		m.send(ctx, b, m.adminID, 0, "Не удалось сохранить группу для уведомлений. Повторите /connectgroup в группе.", nil)
	}
}

func (m *paymentNotificationGroupManager) configureTopic(ctx context.Context, b *bot.Bot, chatID int64, threadID int, responseChatID int64, responseThreadID int) {
	group, ok := m.findGroup(chatID)
	if !ok || !group.IsForum || threadID <= 0 {
		m.send(ctx, b, responseChatID, responseThreadID, "Эта группа с топиками не ожидает настройки. Проверьте /groups.", nil)
		return
	}
	if !m.send(ctx, b, chatID, threadID, "Топик подключён: уведомления об оплате будут приходить сюда.", nil) {
		m.send(ctx, b, responseChatID, responseThreadID, "Не удалось отправить сообщение в этот топик. Проверьте ID и права бота.", nil)
		return
	}
	group.ThreadID = threadID
	if err := m.settings.SaveNotificationGroup(ctx, group); err != nil {
		slog.Error("notification topic registration failed", "error", err, "chat_id", chatID, "thread_id", threadID)
		m.send(ctx, b, responseChatID, responseThreadID, "Не удалось сохранить топик. Попробуйте снова.", nil)
		return
	}
	if responseChatID != chatID {
		m.send(ctx, b, responseChatID, responseThreadID, "Топик сохранён. Уведомления об оплате будут приходить туда.", nil)
	}
}

func (m *paymentNotificationGroupManager) findGroup(chatID int64) (integrations.NotificationGroup, bool) {
	cfg, ok := m.settings.StoredConfig(integrations.ProviderNotificationBot)
	if !ok {
		return integrations.NotificationGroup{}, false
	}
	for _, group := range integrations.ParseNotificationGroups(cfg) {
		if group.ChatID == chatID {
			return group, true
		}
	}
	return integrations.NotificationGroup{}, false
}

func (m *paymentNotificationGroupManager) pendingForumGroups() []integrations.NotificationGroup {
	cfg, ok := m.settings.StoredConfig(integrations.ProviderNotificationBot)
	if !ok {
		return nil
	}
	var pending []integrations.NotificationGroup
	for _, group := range integrations.ParseNotificationGroups(cfg) {
		if group.IsForum && group.ThreadID == 0 {
			pending = append(pending, group)
		}
	}
	return pending
}

func (m *paymentNotificationGroupManager) sendMenu(ctx context.Context, b *bot.Bot, chatID int64) {
	cfg, _ := m.settings.StoredConfig(integrations.ProviderNotificationBot)
	lines := []string{"Уведомления об оплате", "Добавьте бота в группу кнопкой ниже. Обычная группа подключится сразу; в группе с топиками бот попросит ID топика."}
	for _, group := range integrations.ParseNotificationGroups(cfg) {
		status := "подключена"
		if group.IsForum {
			status = "ожидает ID топика"
			if group.ThreadID > 0 {
				status = fmt.Sprintf("топик %d", group.ThreadID)
			}
		}
		lines = append(lines, fmt.Sprintf("• %s — %d (%s)", group.Title, group.ChatID, status))
	}
	lines = append(lines, "Для удаления: /remove ID_группы")
	var keyboard *models.InlineKeyboardMarkup
	if m.username == "" {
		if me, err := b.GetMe(ctx); err == nil {
			m.username = me.Username
		}
	}
	if m.username != "" {
		keyboard = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Добавить в группу", URL: "https://t.me/" + m.username + "?startgroup=payments"}}}}
	}
	m.send(ctx, b, chatID, 0, strings.Join(lines, "\n\n"), keyboard)
}

func (m *paymentNotificationGroupManager) send(ctx context.Context, b *bot.Bot, chatID int64, threadID int, text string, keyboard *models.InlineKeyboardMarkup) bool {
	if chatID == 0 {
		return false
	}
	params := &bot.SendMessageParams{ChatID: chatID, MessageThreadID: threadID, Text: text}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}
	_, err := b.SendMessage(ctx, params)
	if err != nil {
		slog.Warn("notification bot message failed", "error", err, "chat_id", chatID, "thread_id", threadID)
		return false
	}
	return true
}
