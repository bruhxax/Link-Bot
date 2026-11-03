package miniapp

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

func withdrawalSettings(h *Handler) runtimeconfig.TelegramWithdrawalSettings {
	if h.runtimeSettings != nil {
		return h.runtimeSettings.Snapshot().Content.WithdrawalNotification
	}
	return runtimeconfig.DefaultSettings().Content.WithdrawalNotification
}

func withdrawalAmount(cents int64) string {
	rubles := strconv.FormatInt(cents/100, 10)
	for i := len(rubles) - 3; i > 0; i -= 3 {
		rubles = rubles[:i] + " " + rubles[i:]
	}
	if cents%100 == 0 {
		return rubles + " ₽"
	}
	return fmt.Sprintf("%s,%02d ₽", rubles, cents%100)
}

func renderWithdrawalNotification(cfg runtimeconfig.TelegramWithdrawalSettings, item database.BalanceWithdrawal, pendingCount int) string {
	template := cfg.NewText
	switch item.Status {
	case "approved":
		template = cfg.ApprovedText
	case "rejected":
		template = cfg.RejectedText
	}
	username := strings.TrimSpace(item.Username)
	if username == "" {
		username = fmt.Sprintf("ID %d", item.TelegramID)
	} else if !strings.HasPrefix(username, "@") {
		username = "@" + username
	}
	return strings.NewReplacer(
		"{id}", strconv.FormatInt(item.ID, 10),
		"{amount}", html.EscapeString(withdrawalAmount(item.AmountCents)),
		"{username}", html.EscapeString(username),
		"{payout_details}", html.EscapeString(item.PayoutDetails),
		"{pending_count}", strconv.Itoa(pendingCount),
	).Replace(template)
}

func withdrawalKeyboard(cfg runtimeconfig.TelegramWithdrawalSettings, id int64) models.InlineKeyboardMarkup {
	button := func(value runtimeconfig.TelegramButtonSettings, action string) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: value.Text, IconCustomEmojiID: value.IconCustomEmojiID,
			Style: value.Style, CallbackData: fmt.Sprintf("wd:%s:%d", action, id)}
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		button(cfg.ApproveButton, "approve"), button(cfg.RejectButton, "reject"),
	}}}
}

func (h *Handler) RunWithdrawalNotifier(ctx context.Context) {
	if h == nil || h.walletRepository == nil || h.telegramBot == nil || config.GetAdminTelegramId() == 0 {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		h.notifyPendingWithdrawals(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Handler) notifyPendingWithdrawals(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	items, err := h.walletRepository.ListUnnotifiedPendingWithdrawals(checkCtx, 10)
	if err != nil {
		slog.Warn("wallet: list pending withdrawal notifications", "error", err)
		return
	}
	if len(items) == 0 {
		return
	}
	count, err := h.walletRepository.PendingWithdrawalCount(checkCtx)
	if err != nil {
		slog.Warn("wallet: count pending withdrawals", "error", err)
		return
	}
	adminID := config.GetAdminTelegramId()
	cfg := withdrawalSettings(h)
	for _, item := range items {
		messageCtx, messageCancel := context.WithTimeout(ctx, 15*time.Second)
		message, err := h.telegramBot.SendMessage(messageCtx, &bot.SendMessageParams{
			ChatID: adminID, Text: renderWithdrawalNotification(cfg, item, count),
			ParseMode: models.ParseModeHTML, ReplyMarkup: withdrawalKeyboard(cfg, item.ID),
		})
		if err != nil {
			messageCancel()
			slog.Warn("wallet: send withdrawal notification", "withdrawal_id", item.ID, "error", err)
			return
		}
		attached, err := h.walletRepository.AttachWithdrawalNotification(messageCtx, item.ID, adminID, int64(message.ID))
		if err != nil {
			messageCancel()
			slog.Warn("wallet: save withdrawal notification", "withdrawal_id", item.ID, "error", err)
			return
		}
		if !attached {
			_, _ = h.telegramBot.DeleteMessage(messageCtx, &bot.DeleteMessageParams{ChatID: adminID, MessageID: message.ID})
		}
		messageCancel()
	}
}

func parseWithdrawalCallback(data string) (string, int64, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != "wd" || (parts[1] != "approve" && parts[1] != "reject") {
		return "", 0, false
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	return parts[1], id, err == nil && id > 0
}

func (h *Handler) HandleWithdrawalCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.CallbackQuery == nil {
		return
	}
	q := update.CallbackQuery
	answer := func(text string, alert bool) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID, Text: text, ShowAlert: alert})
	}
	if q.Data == "wd:preview" {
		answer("Это тестовое сообщение", false)
		return
	}
	action, id, ok := parseWithdrawalCallback(q.Data)
	if !ok || h.walletRepository == nil || !h.HasAdminPermission(ctx, q.From.ID, "wallet.withdrawals") {
		answer("Недостаточно прав", true)
		return
	}
	message := q.Message.Message
	if message == nil {
		answer("Сообщение недоступно", true)
		return
	}
	item, err := h.walletRepository.GetWithdrawal(ctx, id)
	if err != nil || item == nil || item.NotificationChatID == nil || item.NotificationMessageID == nil ||
		*item.NotificationChatID != message.Chat.ID || *item.NotificationMessageID != int64(message.ID) {
		answer("Заявка не найдена", true)
		return
	}
	if item.Status != "pending" {
		_ = h.refreshWithdrawalNotification(ctx, item)
		answer("Заявка уже обработана", true)
		return
	}
	if err := h.walletRepository.ResolveWithdrawal(ctx, id, q.From.ID, action == "approve"); err != nil {
		if errors.Is(err, database.ErrWithdrawalAlreadyResolved) {
			answer("Заявка уже обработана", true)
		} else {
			slog.Error("wallet: resolve withdrawal callback", "withdrawal_id", id, "error", err)
			answer("Не удалось обработать заявку", true)
		}
		return
	}
	item, err = h.walletRepository.GetWithdrawal(ctx, id)
	if err == nil && item != nil {
		if err := h.refreshWithdrawalNotification(ctx, item); err != nil {
			slog.Warn("wallet: refresh withdrawal notification", "withdrawal_id", id, "error", err)
		}
	}
	if action == "approve" {
		answer("Выплата отмечена", false)
	} else {
		answer("Заявка отклонена, средства возвращены", false)
	}
}

func (h *Handler) refreshWithdrawalNotification(ctx context.Context, item *database.BalanceWithdrawal) error {
	if h.telegramBot == nil || item == nil || item.NotificationChatID == nil || item.NotificationMessageID == nil {
		return nil
	}
	count, err := h.walletRepository.PendingWithdrawalCount(ctx)
	if err != nil {
		return err
	}
	_, err = h.telegramBot.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: *item.NotificationChatID, MessageID: int(*item.NotificationMessageID),
		Text:        renderWithdrawalNotification(withdrawalSettings(h), *item, count),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
	})
	return err
}
