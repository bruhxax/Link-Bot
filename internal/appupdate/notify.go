package appupdate

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type NotificationStore interface {
	Claim(context.Context, string) (bool, error)
	Finish(context.Context, string, bool) error
}

func NotificationRevision(status Status, currentCommit string) string {
	if status.State != "available" || status.LatestCommit == "" || !CommitPattern.MatchString(status.LatestCommit) {
		return ""
	}
	return strings.Join([]string{currentCommit, status.LatestCommit, status.LatestVersion}, "|")
}

func NotificationText(status Status, currentVersion string) string {
	var text strings.Builder
	if status.Kind == "release" {
		fmt.Fprintf(&text, "🆕 <b>Новая версия Link-Bot %s</b>\n", html.EscapeString(status.LatestVersion))
	} else {
		text.WriteString("🔄 <b>Новые изменения Link-Bot</b>\n")
	}
	fmt.Fprintf(&text, "\nУстановлено: <code>%s</code>", html.EscapeString(currentVersion))
	if status.AheadBy > 0 {
		fmt.Fprintf(&text, "\nНовых коммитов: <b>%d</b>", status.AheadBy)
	}
	if CommitPattern.MatchString(status.LatestCommit) {
		commit := status.LatestCommit
		if len(commit) > 8 {
			commit = commit[:8]
		}
		fmt.Fprintf(&text, "\nПоследний коммит: <code>%s</code>", commit)
	}
	if status.Summary != "" {
		runes := []rune(status.Summary)
		if len(runes) > 240 {
			runes = runes[:240]
		}
		fmt.Fprintf(&text, "\n%s", html.EscapeString(string(runes)))
	}
	text.WriteString("\n\nОбновление: <b>Админка → Статус → Обновить бота</b>.")
	return text.String()
}

// NotifyOnce is separate from customer broadcasts and addresses only the owner.
func NotifyOnce(ctx context.Context, status Status, version, commit string, store NotificationStore, send func(context.Context, string, string) error) error {
	revision := NotificationRevision(status, commit)
	if revision == "" {
		return nil
	}
	claimed, err := store.Claim(ctx, revision)
	if err != nil || !claimed {
		return err
	}
	delivered := false
	defer func() {
		finishCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.Finish(finishCtx, revision, delivered); err != nil {
			slog.Warn("update notification checkpoint failed", "error", err)
		}
	}()
	if err := send(ctx, NotificationText(status, version), status.URL); err != nil {
		return err
	}
	delivered = true
	return nil
}

func RunNotifications(ctx context.Context, b *bot.Bot, owner int64, store NotificationStore, version, commit string) {
	if b == nil || owner <= 0 || !CommitPattern.MatchString(commit) {
		return
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			status := Check(checkCtx, &http.Client{Timeout: 6 * time.Second}, version, commit)
			err := NotifyOnce(checkCtx, status, version, commit, store, func(sendCtx context.Context, text, url string) error {
				disablePreview := true
				_, err := b.SendMessage(sendCtx, &bot.SendMessageParams{ChatID: owner, Text: text, ParseMode: models.ParseModeHTML, LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: &disablePreview}, ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Посмотреть изменения", URL: url}}}}})
				return err
			})
			cancel()
			if err != nil {
				slog.Warn("update availability notification failed", "error", err)
			}
			timer.Reset(10 * time.Minute)
		}
	}
}
