package handler

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

type inlineAdminCommand struct {
	Kind   string
	Search string
	Value  int64
}

func isInlineAdminNumericQuery(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func parseInlineAdminCommand(raw string) (inlineAdminCommand, bool) {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) == 0 {
		return inlineAdminCommand{}, false
	}
	command := inlineAdminCommand{}
	switch strings.ToLower(parts[0]) {
	case "user", "users", "find", "юзер", "пользователь", "найти":
		command.Kind = "user"
		command.Search = strings.Join(parts[1:], " ")
	case "extend", "продлить":
		command.Kind = "extend"
	case "balance", "баланс":
		command.Kind = "balance"
	case "promo", "промо", "промокод":
		command.Kind = "promo"
		command.Search = strings.Join(parts[1:], " ")
	default:
		return inlineAdminCommand{}, false
	}
	if command.Kind == "extend" || command.Kind == "balance" {
		if len(parts) < 2 {
			return command, true
		}
		command.Value, _ = strconv.ParseInt(parts[1], 10, 64)
		if len(parts) > 2 {
			command.Search = strings.Join(parts[2:], " ")
		}
	}
	return command, true
}

func inlineAdminName(user database.AdminUserSummary) string {
	if user.TelegramUsername != "" {
		return "@" + user.TelegramUsername
	}
	return strconv.FormatInt(user.TelegramID, 10)
}

func inlineAdminSubscription(user database.AdminUserSummary) string {
	if user.SubscriptionCount == 0 || user.ExpireAt == nil {
		return "Подписка не найдена"
	}
	status := "Истекла"
	if user.IsBlocked {
		status = "Заблокирована"
	} else if user.ExpireAt.After(time.Now().UTC()) {
		status = "Активна"
	}
	name := strings.TrimSpace(user.SubscriptionName)
	if name == "" {
		name = "Подписка"
	}
	return fmt.Sprintf("%s · %s до %s", name, status, user.ExpireAt.In(time.FixedZone("MSK", 3*3600)).Format("02.01.2006"))
}

func inlineAdminPanelURL(customerID int64) string {
	base := config.AdminURL("users")
	if strings.HasPrefix(base, "/") {
		base = strings.TrimRight(config.PublicBaseURL(), "/") + base
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	query := parsed.Query()
	query.Set("inlineUser", strconv.FormatInt(customerID, 10))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func inlineAdminBotURL(customerID int64) string {
	return strings.TrimRight(config.BotURL(), "/") + "?start=au_" + strconv.FormatInt(customerID, 10)
}

func inlineAdminHelp(kind string) string {
	switch kind {
	case "user":
		return "Введите: пользователь @ник или Telegram ID"
	case "extend":
		return "Введите: продлить 30 @ник или Telegram ID"
	case "balance":
		return "Введите: баланс 100 @ник или Telegram ID"
	default:
		return "Введите: промо КОД"
	}
}

func (h *Handler) answerInlineAdmin(ctx context.Context, b *bot.Bot, query *models.InlineQuery, command inlineAdminCommand) {
	if h.inlineAdmin == nil || query.From == nil {
		return
	}
	permission := "users"
	switch command.Kind {
	case "extend":
		permission = "users.subscription"
	case "balance":
		permission = "users.balance"
	case "promo":
		permission = "promocodes"
	}
	results := make([]models.InlineQueryResult, 0, 16)
	allowed := h.inlineAdmin.HasAdminPermission(ctx, query.From.ID, permission)
	if allowed {
		if command.Kind == "promo" && command.Search != "" {
			code, err := h.inlineAdmin.FindInlinePromo(ctx, query.From.ID, command.Search)
			if err == nil {
				results = append(results, &models.InlineQueryResultArticle{ID: "promo-" + code, Title: "Промокод " + code,
					Description: "Отправить промокод в чат", InputMessageContent: models.InputTextMessageContent{
						MessageText: "🎁 Промокод: <code>" + html.EscapeString(code) + "</code>", ParseMode: models.ParseModeHTML,
					}})
			}
		} else if command.Kind != "promo" && command.Search != "" && len([]rune(command.Search)) <= 64 &&
			(command.Kind == "user" || command.Value >= 1 && (command.Kind == "extend" && command.Value <= 3650 || command.Kind == "balance" && command.Value <= 1000000)) {
			users, _, err := h.customerRepository.SearchAdminUsers(ctx, command.Search, 15, 0)
			if err != nil {
				slog.Error("inline admin search failed", "error", err)
			}
			for _, user := range users {
				name := inlineAdminName(user)
				summary := inlineAdminSubscription(user)
				title := name
				message := "👤 <b>" + html.EscapeString(name) + "</b>\nTelegram ID: <code>" + strconv.FormatInt(user.TelegramID, 10) + "</code>\n" + html.EscapeString(summary)
				keyboard := models.InlineKeyboardMarkup{}
				switch command.Kind {
				case "user":
					buttons := []models.InlineKeyboardButton{{Text: "Открыть в боте", URL: inlineAdminBotURL(user.CustomerID)}}
					if panel := inlineAdminPanelURL(user.CustomerID); panel != "" {
						buttons = append(buttons, models.InlineKeyboardButton{Text: "Открыть в панели", URL: panel})
					}
					keyboard.InlineKeyboard = [][]models.InlineKeyboardButton{buttons}
				case "extend":
					if user.IsBlocked || user.ExpireAt == nil || user.SubscriptionCount == 0 {
						continue
					}
					// The selected subscription ID is resolved by a fresh customer lookup
					// below, never inferred from the display name or Telegram ID.
					subscriptionID, err := h.inlineAdminSubscriptionID(ctx, user.CustomerID)
					if err != nil || subscriptionID == 0 {
						continue
					}
					base := time.Now().UTC()
					if user.ExpireAt.After(base) {
						base = *user.ExpireAt
					}
					target := base.Add(time.Duration(command.Value) * 24 * time.Hour).Unix()
					message += fmt.Sprintf("\n\nПродление на <b>%d дн.</b> ожидает подтверждения администратора.", command.Value)
					keyboard.InlineKeyboard = [][]models.InlineKeyboardButton{{{Text: fmt.Sprintf("Подтвердить +%d дн.", command.Value), CallbackData: fmt.Sprintf("ia:e:%d:%d:%d:%d", query.From.ID, user.CustomerID, subscriptionID, target)}}}
					title = fmt.Sprintf("Продлить %s на %d дн.", name, command.Value)
				case "balance":
					nonce := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
					message += fmt.Sprintf("\n\nПополнение баланса на <b>%d ₽</b> ожидает подтверждения администратора.", command.Value)
					keyboard.InlineKeyboard = [][]models.InlineKeyboardButton{{{Text: fmt.Sprintf("Подтвердить +%d ₽", command.Value), CallbackData: fmt.Sprintf("ia:b:%d:%d:%d:%s", query.From.ID, user.CustomerID, command.Value, nonce)}}}
					title = fmt.Sprintf("Пополнить %s на %d ₽", name, command.Value)
				}
				results = append(results, &models.InlineQueryResultArticle{ID: fmt.Sprintf("%s-%d", command.Kind, user.CustomerID), Title: title,
					Description: summary, InputMessageContent: models.InputTextMessageContent{MessageText: message, ParseMode: models.ParseModeHTML}, ReplyMarkup: keyboard})
			}
		}
	}
	if len(results) == 0 && allowed {
		help := inlineAdminHelp(command.Kind)
		results = append(results, &models.InlineQueryResultArticle{ID: "admin-help", Title: help,
			Description: "Админский inline-поиск", InputMessageContent: models.InputTextMessageContent{MessageText: help}})
	}
	if _, err := b.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{InlineQueryID: query.ID, Results: results, IsPersonal: true, CacheTime: 0}); err != nil {
		slog.Error("inline admin response failed", "error", err)
	}
}

func (h *Handler) inlineAdminSubscriptionID(ctx context.Context, customerID int64) (int64, error) {
	// The repository search reports the selected subscription but not its ID.
	// Resolve the current selection using the same method as the admin panel.
	return h.inlineAdmin.SelectedInlineSubscriptionID(ctx, customerID)
}

func (h *Handler) InlineAdminActionHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery
	if callback == nil || h.inlineAdmin == nil {
		return
	}
	answer := func(message string) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callback.ID, Text: message, ShowAlert: message != ""})
	}
	if callback.Data == "ia:done" {
		answer("Уже выполнено")
		return
	}
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 6 || parts[0] != "ia" || callback.InlineMessageID == "" {
		answer("Действие недоступно")
		return
	}
	actor, aErr := strconv.ParseInt(parts[2], 10, 64)
	customerID, cErr := strconv.ParseInt(parts[3], 10, 64)
	value, vErr := strconv.ParseInt(parts[4], 10, 64)
	if aErr != nil || cErr != nil || vErr != nil || actor != callback.From.ID || customerID <= 0 {
		answer("Подтвердить может только отправитель")
		return
	}
	var err error
	switch parts[1] {
	case "e":
		var target int64
		target, err = strconv.ParseInt(parts[5], 10, 64)
		if err == nil {
			err = h.inlineAdmin.ExtendInlineUser(ctx, actor, customerID, value, time.Unix(target, 0))
		}
	case "b":
		err = h.inlineAdmin.CreditInlineUser(ctx, actor, customerID, value, parts[5])
	default:
		answer("Действие недоступно")
		return
	}
	if err != nil {
		slog.Warn("inline admin action failed", "kind", parts[1], "error", err)
		answer("Не выполнено: " + err.Error())
		return
	}
	answer("Готово")
	_, _ = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{InlineMessageID: callback.InlineMessageID,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "✓ Выполнено", CallbackData: "ia:done"}}}}})
}

func (h *Handler) sendInlineAdminUserEntry(ctx context.Context, b *bot.Bot, actorID int64, start string) {
	if h.inlineAdmin == nil || !h.inlineAdmin.HasAdminPermission(ctx, actorID, "users") {
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(start, "au_"), 10, 64)
	if err != nil || id <= 0 {
		return
	}
	user, err := h.customerRepository.FindById(ctx, id)
	if err != nil || user == nil {
		return
	}
	name := strconv.FormatInt(user.TelegramID, 10)
	if user.TelegramUsername != nil && *user.TelegramUsername != "" {
		name = "@" + *user.TelegramUsername
	}
	text := "👤 <b>" + html.EscapeString(name) + "</b>\nTelegram ID: <code>" + strconv.FormatInt(user.TelegramID, 10) + "</code>"
	if rows, _, lookupErr := h.customerRepository.SearchAdminUsers(ctx, strconv.FormatInt(user.TelegramID, 10), 15, 0); lookupErr == nil {
		for _, row := range rows {
			if row.CustomerID == id {
				text += "\n" + html.EscapeString(inlineAdminSubscription(row))
				break
			}
		}
	}
	var markup *models.InlineKeyboardMarkup
	if panel := inlineAdminPanelURL(id); panel != "" {
		markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Открыть в панели", URL: panel}}}}
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: actorID, Text: text, ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
}
