package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/database"
)

const alertCooldown = 15 * time.Minute

var sensitiveValuePattern = regexp.MustCompile(`(?i)(token|secret|password|authorization|api[_-]?key)([=: ]+)([^\s&]+)`)

type Reporter struct {
	repository *database.RuntimeSettingsRepository
	bot        *bot.Bot
	adminID    int64

	mu        sync.Mutex
	lastAlert map[string]time.Time
}

type ReportInput struct {
	Category  string
	Severity  string
	Operation string
	Message   string
	Err       error
	Details   map[string]interface{}
}

func NewReporter(repository *database.RuntimeSettingsRepository, telegramBot *bot.Bot, adminID int64) *Reporter {
	return &Reporter{
		repository: repository,
		bot:        telegramBot,
		adminID:    adminID,
		lastAlert:  map[string]time.Time{},
	}
}

func (r *Reporter) Report(ctx context.Context, input ReportInput) {
	if r == nil {
		return
	}
	input.Category = normalizeLabel(input.Category, "system")
	input.Severity = normalizeSeverity(input.Severity)
	input.Operation = normalizeLabel(input.Operation, "unknown")
	input.Message = cleanMessage(input.Message)
	if input.Message == "" && input.Err != nil {
		input.Message = cleanMessage(input.Err.Error())
	}
	if input.Message == "" {
		input.Message = "Неизвестная ошибка"
	}

	fingerprint := eventFingerprint(input)
	details := cloneDetails(input.Details)
	if input.Err != nil {
		details["error"] = cleanMessage(input.Err.Error())
	}
	detailsRaw, _ := json.Marshal(details)

	event := &database.OperationalEvent{
		Fingerprint:     fingerprint,
		Category:        input.Category,
		Severity:        input.Severity,
		Operation:       input.Operation,
		Message:         input.Message,
		OccurrenceCount: 1,
		FirstSeenAt:     time.Now().UTC(),
		LastSeenAt:      time.Now().UTC(),
	}
	if r.repository != nil {
		stored, err := r.repository.RecordOperationalEvent(ctx, database.OperationalEventInput{
			Fingerprint: fingerprint,
			Category:    input.Category,
			Severity:    input.Severity,
			Operation:   input.Operation,
			Message:     input.Message,
			Details:     detailsRaw,
		})
		if err != nil {
			slog.Error("operations: persist event failed", "error", err, "category", input.Category, "operation", input.Operation)
		} else {
			event = stored
		}
	}

	if !r.shouldAlert(fingerprint, event.OccurrenceCount, input.Severity) {
		return
	}
	r.sendAdminAlert(ctx, event)
}

func (r *Reporter) List(ctx context.Context, limit int, includeResolved bool) ([]database.OperationalEvent, error) {
	if r == nil || r.repository == nil {
		return []database.OperationalEvent{}, nil
	}
	return r.repository.ListOperationalEvents(ctx, limit, includeResolved)
}

func (r *Reporter) Resolve(ctx context.Context, id int64) error {
	if r == nil || r.repository == nil {
		return nil
	}
	return r.repository.ResolveOperationalEvent(ctx, id)
}

func (r *Reporter) shouldAlert(fingerprint string, count int, severity string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	last := r.lastAlert[fingerprint]
	importantCount := count == 1 || count == 3 || count == 10 || count%25 == 0
	if severity == "critical" || importantCount || now.Sub(last) >= alertCooldown {
		r.lastAlert[fingerprint] = now
		return true
	}
	return false
}

func (r *Reporter) sendAdminAlert(_ context.Context, event *database.OperationalEvent) {
	if r.bot == nil || r.adminID == 0 || event == nil {
		return
	}
	icon := "⚠️"
	severity := "Предупреждение"
	if event.Severity == "critical" {
		icon = "🚨"
		severity = "Критическая ошибка"
	} else if event.Severity == "info" {
		icon = "ℹ️"
		severity = "Событие"
	}

	text := fmt.Sprintf(
		"%s <b>%s</b>\n\n"+
			"<b>Раздел:</b> %s\n"+
			"<b>Операция:</b> %s\n"+
			"<b>Ошибка:</b> %s\n"+
			"<b>Повторений:</b> %d\n"+
			"<b>Время:</b> %s\n\n"+
			"Событие сохранено в диагностике админки.",
		icon,
		html.EscapeString(severity),
		html.EscapeString(event.Category),
		html.EscapeString(event.Operation),
		html.EscapeString(event.Message),
		event.OccurrenceCount,
		event.LastSeenAt.Local().Format("02.01.2006 15:04:05"),
	)

	alertCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err := r.bot.SendMessage(alertCtx, &bot.SendMessageParams{
		ChatID:    r.adminID,
		ParseMode: models.ParseModeHTML,
		Text:      text,
	})
	if err != nil {
		slog.Error("operations: admin alert failed", "error", err, "eventId", event.ID)
	}
}

func eventFingerprint(input ReportInput) string {
	raw := strings.Join([]string{
		strings.ToLower(input.Category),
		strings.ToLower(input.Operation),
		strings.ToLower(cleanMessage(input.Message)),
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}

func normalizeLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	runes := []rune(value)
	if len(runes) > 80 {
		value = string(runes[:80])
	}
	return value
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return "critical"
	case "info":
		return "info"
	default:
		return "warning"
	}
}

func cleanMessage(value string) string {
	value = sensitiveValuePattern.ReplaceAllString(value, "$1$2<redacted>")
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 500 {
		value = string(runes[:500]) + "…"
	}
	return value
}

func cloneDetails(value map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, item := range value {
		key = normalizeLabel(key, "detail")
		switch typed := item.(type) {
		case string:
			result[key] = cleanMessage(typed)
		case bool, int, int32, int64, float32, float64:
			result[key] = typed
		default:
			result[key] = cleanMessage(fmt.Sprint(typed))
		}
	}
	return result
}
