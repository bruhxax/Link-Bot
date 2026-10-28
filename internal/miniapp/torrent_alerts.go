package miniapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/database"
)

type torrentAlertRepository interface {
	RecordTorrentAlert(context.Context, string, uuid.UUID, int64, database.UserActivity) error
	ClaimTorrentAlert(context.Context, string, uuid.UUID) (bool, bool, error)
	FinishTorrentAlert(context.Context, string, uuid.UUID, bool) error
}

// Only these fields are decoded. Full user/node objects include credentials and
// subscription URLs and must never be stored or copied into notifications.
type torrentReport struct {
	Node struct {
		UUID uuid.UUID `json:"uuid"`
		Name string    `json:"name"`
	} `json:"node"`
	User struct {
		UUID     uuid.UUID `json:"uuid"`
		ID       int64     `json:"id"`
		Username string    `json:"username"`
	} `json:"user"`
	Report struct {
		Action struct {
			Blocked       *bool  `json:"blocked"`
			IP            string `json:"ip"`
			BlockDuration int64  `json:"blockDuration"`
			WillUnblockAt string `json:"willUnblockAt"`
			ProcessedAt   string `json:"processedAt"`
		} `json:"actionReport"`
		Xray struct {
			Protocol string `json:"protocol"`
			Network  string `json:"network"`
			TS       int64  `json:"ts"`
		} `json:"xrayReport"`
	} `json:"report"`
}

func parseTorrentReport(raw json.RawMessage) (torrentReport, time.Time, error) {
	var report torrentReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return report, time.Time{}, err
	}
	action := report.Report.Action
	processed, err := parseTorrentTime(action.ProcessedAt)
	if err != nil || report.Node.UUID == uuid.Nil || report.User.UUID == uuid.Nil || action.Blocked == nil || net.ParseIP(action.IP) == nil || action.BlockDuration < 0 {
		return report, time.Time{}, errors.New("invalid torrent report identity or action")
	}
	if *action.Blocked {
		if _, err := parseTorrentTime(action.WillUnblockAt); err != nil {
			return report, time.Time{}, errors.New("invalid torrent unblock time")
		}
	}
	return report, processed, nil
}

func parseTorrentTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid torrent timestamp")
}

func torrentEventKey(report torrentReport, processed time.Time) string {
	// A redelivered report can have a new envelope timestamp or JSON field order.
	// Its panel identities and processing time remain the same.
	identity, _ := json.Marshal([]any{report.Node.UUID, report.User.UUID, net.ParseIP(report.Report.Action.IP).String(), processed.UnixNano(), report.Report.Xray.TS, *report.Report.Action.Blocked})
	return fmt.Sprintf("%x", sha256.Sum256(identity))
}

func torrentReportDetails(report torrentReport) []database.UserActivityDetail {
	action := report.Report.Action
	result := "IP не заблокирован"
	if *action.Blocked {
		result = "IP заблокирован на ноде"
	}
	details := []database.UserActivityDetail{
		{Label: "Пользователь панели", Value: report.User.Username},
		{Label: "UUID пользователя", Value: report.User.UUID.String()},
		{Label: "Нода", Value: report.Node.Name},
		{Label: "IP", Value: action.IP},
		{Label: "Результат", Value: result},
	}
	if *action.Blocked {
		until, _ := parseTorrentTime(action.WillUnblockAt)
		details = append(details, database.UserActivityDetail{Label: "Срок блокировки", Value: fmt.Sprintf("%d сек.", action.BlockDuration)},
			database.UserActivityDetail{Label: "Снятие блокировки", Value: formatSecurityTime(until)})
	}
	for _, item := range []database.UserActivityDetail{
		{Label: "Протокол", Value: report.Report.Xray.Protocol},
		{Label: "Сеть", Value: report.Report.Xray.Network},
	} {
		if strings.TrimSpace(item.Value) != "" {
			details = append(details, item)
		}
	}
	// Bound display fields before persistence, without storing arbitrary Xray data.
	for i := range details {
		value := []rune(strings.TrimSpace(details[i].Value))
		if len(value) > 160 {
			value = append(value[:160], '…')
		}
		details[i].Value = string(value)
	}
	return details
}

func formatTorrentAlert(report torrentReport, occurred time.Time) string {
	message := "🧲 <b>Обнаружен торрент-трафик</b>\n\n"
	for _, detail := range torrentReportDetails(report) {
		message += safeSecurityText(detail.Label) + ": <code>" + safeSecurityText(detail.Value) + "</code>\n"
	}
	return message + "🕒 Время: " + formatSecurityTime(occurred)
}

func (h *Handler) handleTorrentReport(w http.ResponseWriter, r *http.Request, event remnawaveSecurityEvent) {
	report, occurred, err := parseTorrentReport(event.Data)
	if err != nil {
		http.Error(w, "invalid torrent report", http.StatusBadRequest)
		return
	}
	repo := h.torrentAlertRepository
	if repo == nil && h.customerRepository != nil {
		repo = h.customerRepository
	}
	if repo == nil {
		http.Error(w, "torrent alerts unavailable", http.StatusServiceUnavailable)
		return
	}
	key := torrentEventKey(report, occurred)
	entry := database.UserActivity{Title: "Обнаружен торрент-трафик", Details: torrentReportDetails(report), CreatedAt: occurred}
	if err := repo.RecordTorrentAlert(r.Context(), key, report.User.UUID, report.User.ID, entry); err != nil {
		slog.Warn("torrent report persistence failed", "error", err)
		http.Error(w, "report persistence failed", http.StatusServiceUnavailable)
		return
	}
	token := uuid.New()
	claimed, delivered, err := repo.ClaimTorrentAlert(r.Context(), key, token)
	if err != nil || !claimed && !delivered {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "notification pending", http.StatusServiceUnavailable)
		return
	}
	if delivered {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	err = h.sendSecurityAlert(ctx, formatTorrentAlert(report, occurred))
	cancel()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer finishCancel()
	finishErr := repo.FinishTorrentAlert(finishCtx, key, token, err == nil)
	if err != nil || finishErr != nil {
		slog.Warn("torrent alert notification failed", "send_error", err, "save_error", finishErr)
		http.Error(w, "notification failed", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
