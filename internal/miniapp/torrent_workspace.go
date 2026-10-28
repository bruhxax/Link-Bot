package miniapp

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

type torrentWorkspaceStore interface {
	SaveTorrentReport(context.Context, database.TorrentReport) error
	ListTorrentReports(context.Context, string, string, int64) ([]database.TorrentReport, bool, database.TorrentSummary, error)
	TorrentRecipients(context.Context, uuid.UUID, int64) ([]int64, error)
	ClaimTorrentUserAlert(context.Context, string, int64, uuid.UUID) (bool, bool, error)
	FinishTorrentUserAlert(context.Context, string, int64, uuid.UUID, bool) error
}

func (h *Handler) torrentWorkspaceStore() torrentWorkspaceStore {
	if repo, ok := h.torrentAlertRepository.(torrentWorkspaceStore); ok {
		return repo
	}
	if h.customerRepository != nil {
		return h.customerRepository
	}
	return nil
}
func (h *Handler) torrentSettings() runtimeconfig.TorrentSettings {
	if h.runtimeSettings != nil {
		return h.runtimeSettings.Snapshot().Torrents
	}
	return runtimeconfig.DefaultTorrentSettings()
}
func torrentDuration(seconds int64) string {
	if seconds%86400 == 0 && seconds > 0 {
		return fmt.Sprintf("%d дн.", seconds/86400)
	}
	if seconds%3600 == 0 && seconds > 0 {
		return fmt.Sprintf("%d ч.", seconds/3600)
	}
	if seconds%60 == 0 && seconds > 0 {
		return fmt.Sprintf("%d мин.", seconds/60)
	}
	return fmt.Sprintf("%d сек.", seconds)
}
func renderTorrentTemplate(template string, report torrentReport, occurred time.Time, user bool) string {
	until := "—"
	if parsed, err := parseTorrentTime(report.Report.Action.WillUnblockAt); err == nil && *report.Report.Action.Blocked {
		until = formatSecurityTime(parsed)
	}
	status := "IP не заблокирован"
	if *report.Report.Action.Blocked {
		status = "IP заблокирован на ноде"
	}
	values := map[string]string{"node": report.Node.Name, "username": report.User.Username, "duration": torrentDuration(report.Report.Action.BlockDuration), "unblock_at": until, "status": status, "time": formatSecurityTime(occurred)}
	if !user {
		values["ip"] = report.Report.Action.IP
		values["user_uuid"] = report.User.UUID.String()
	}
	for key, value := range values {
		runes := []rune(value)
		if len(runes) > 160 {
			values[key] = string(runes[:160])
		}
	}
	return runtimeconfig.ReplaceTorrentVariables(template, values, html.EscapeString)
}
func (h *Handler) handleAdminTorrentReports(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.canAdmin("torrents") {
		h.writeError(w, 403, "forbidden", "Нет доступа к торрент-отчётам")
		return
	}
	var req struct {
		Query    string `json:"query"`
		Status   string `json:"status"`
		BeforeID int64  `json:"beforeId"`
	}
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil || len([]rune(req.Query)) > 160 || req.BeforeID < 0 {
		h.writeError(w, 400, "invalid_request", "Некорректный поиск")
		return
	}
	if req.Status == "" {
		req.Status = "all"
	}
	if req.Status != "all" && req.Status != "active" && req.Status != "released" && req.Status != "observed" {
		h.writeError(w, 400, "invalid_request", "Неизвестный фильтр")
		return
	}
	repo := h.torrentWorkspaceStore()
	if repo == nil {
		h.writeError(w, 503, "unavailable", "Отчёты временно недоступны")
		return
	}
	items, more, summary, err := repo.ListTorrentReports(r.Context(), strings.TrimSpace(req.Query), req.Status, req.BeforeID)
	if err != nil {
		h.writeError(w, 503, "unavailable", "Не удалось загрузить отчёты")
		return
	}
	if !sess.canAdmin("users") {
		for i := range items {
			items[i].CustomerID = 0
		}
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"items": items, "hasMore": more, "summary": summary, "webhookConfigured": len(strings.TrimSpace(os.Getenv("REMNAWAVE_WEBHOOK_SECRET"))) >= 32}})
}
func (h *Handler) sendTorrentUserAlerts(ctx context.Context, key string, report torrentReport, occurred time.Time) error {
	settings := h.torrentSettings()
	if !settings.UserEnabled || !*report.Report.Action.Blocked {
		return nil
	}
	if until, err := parseTorrentTime(report.Report.Action.WillUnblockAt); err == nil && !until.After(time.Now()) {
		return nil
	}
	repo := h.torrentWorkspaceStore()
	if repo == nil {
		return nil
	}
	ids, err := repo.TorrentRecipients(ctx, report.User.UUID, report.User.ID)
	if err != nil {
		return err
	}
	var firstError error
	for _, id := range ids {
		token := uuid.New()
		claimed, delivered, err := repo.ClaimTorrentUserAlert(ctx, key, id, token)
		if err != nil || !claimed && !delivered {
			if firstError == nil {
				firstError = fmt.Errorf("user alert pending")
			}
			continue
		}
		if delivered {
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, sendErr := h.telegramBot.SendMessage(sendCtx, &bot.SendMessageParams{ChatID: id, Text: renderTorrentTemplate(settings.UserText, report, occurred, true), ParseMode: models.ParseModeHTML})
		cancel()
		// A user who blocked the bot cannot receive this report. Complete that
		// permanent result without retrying the administrator's notification.
		complete := sendErr == nil || errors.Is(sendErr, bot.ErrorForbidden)
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		finishErr := repo.FinishTorrentUserAlert(finishCtx, key, id, token, complete)
		finishCancel()
		if firstError == nil {
			if finishErr != nil {
				firstError = finishErr
			} else if !complete {
				firstError = sendErr
			}
		}
	}
	return firstError
}
