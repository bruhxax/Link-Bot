package miniapp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"

	"link-bot/internal/database"
)

var subscriptionReissueWorkerLock sync.Mutex

func (h *Handler) handleAdminUserReissueSubscription(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req adminUserActionRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.CustomerID <= 0 || req.SubscriptionID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите подписку")
		return
	}
	// Serialize panel creation and the database switch within this bot process.
	adminSubscriptionRebindLock.Lock()
	defer adminSubscriptionRebindLock.Unlock()
	target, subscription, err := h.adminUserSubscriptionTarget(r.Context(), req.CustomerID, req.SubscriptionID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "admin_subscription_unavailable", err.Error())
		return
	}
	if target.customer.IsBlocked || target.customer.TelegramIDIsSynthetic {
		h.writeError(w, http.StatusConflict, "admin_reissue_unavailable", "Пользователь заблокирован или не привязан к Telegram")
		return
	}
	if !target.state.Active {
		h.writeError(w, http.StatusConflict, "admin_reissue_inactive", "Можно перевыпустить только активную подписку")
		return
	}
	old, created, err := h.remnawaveClient.ReissueUser(r.Context(), target.state.UserID, target.state.UserUUID)
	if err != nil {
		if created != nil {
			h.removeFailedReissue(created.ID, created.UUID)
		}
		slog.Error("mini app: reissue panel subscription", "error", err, "subscriptionId", subscription.ID)
		h.writeError(w, http.StatusBadGateway, "admin_reissue_failed", "Не удалось перевыпустить подписку в панели")
		return
	}
	link := strings.TrimSpace(created.SubscriptionURL)
	if err = h.subscriptionRepository.SwapPanelAccessForReissue(r.Context(), subscription, old.ID, old.UUID, created.ID, created.UUID, link, old.ExpireAt, target.customer.TelegramID); err != nil {
		h.removeFailedReissue(created.ID, created.UUID)
		slog.Error("mini app: save reissued subscription", "error", err, "subscriptionId", subscription.ID)
		status := http.StatusInternalServerError
		if errors.Is(err, database.ErrSubscriptionReissueConflict) {
			status = http.StatusConflict
		}
		h.writeError(w, status, "admin_reissue_sync_failed", "Подписка изменилась, повторите попытку")
		return
	}
	// Delivery is retried by the persistent worker if Telegram is temporarily unavailable.
	h.processPendingSubscriptionReissues(r.Context())
	h.writeAdminUserActionResult(w, r, req.CustomerID, "Подписка перевыпущена. Старая ссылка действует ещё 10 минут")
}

func (h *Handler) removeFailedReissue(userID int64, userUUID uuid.UUID) {
	if userID <= 0 && userUUID == uuid.Nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.remnawaveClient.DeleteUser(ctx, userID, userUUID); err != nil {
		slog.Error("mini app: remove orphaned reissue", "error", err)
	}
}

func (h *Handler) StartSubscriptionReissueCleaner(ctx context.Context) {
	if h == nil || h.subscriptionRepository == nil || h.remnawaveClient == nil || h.telegramBot == nil {
		return
	}
	go func() {
		h.processPendingSubscriptionReissues(ctx)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.processPendingSubscriptionReissues(ctx)
			}
		}
	}()
}

func (h *Handler) processPendingSubscriptionReissues(ctx context.Context) {
	subscriptionReissueWorkerLock.Lock()
	defer subscriptionReissueWorkerLock.Unlock()
	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	items, err := h.subscriptionRepository.ListPendingReissues(checkCtx, 50)
	if err != nil {
		slog.Warn("mini app: load subscription reissues", "error", err)
		return
	}
	for _, item := range items {
		if checkCtx.Err() != nil {
			return
		}
		if item.NotifiedAt == nil && (item.LastNotificationAttemptAt == nil || time.Since(*item.LastNotificationAttemptAt) >= 5*time.Minute) {
			message := "🔄 Ваша подписка перевыпущена. Добавьте новую ссылку в VPN-клиент:\n" + item.NewLink + "\n\nСтарая ссылка будет работать ещё 10 минут после перевыпуска, затем отключится."
			if err := h.subscriptionRepository.MarkReissueNotificationAttempt(checkCtx, item.ID); err != nil {
				slog.Warn("mini app: mark reissue notification attempt", "error", err, "reissueId", item.ID)
			} else if _, err := h.telegramBot.SendMessage(checkCtx, &bot.SendMessageParams{ChatID: item.TelegramID, Text: message}); err != nil {
				slog.Warn("mini app: notify subscription reissue", "error", err, "reissueId", item.ID)
			} else if err := h.subscriptionRepository.MarkReissueNotified(checkCtx, item.ID); err != nil {
				slog.Warn("mini app: mark reissue notified", "error", err, "reissueId", item.ID)
			}
		}
		if item.DeletedAt != nil || time.Now().UTC().Before(item.DeleteAfter) {
			continue
		}
		oldID := int64(0)
		oldUUID := uuid.Nil
		if item.OldPanelUserID != nil {
			oldID = *item.OldPanelUserID
		}
		if item.OldPanelUUID != nil {
			oldUUID = *item.OldPanelUUID
		}
		if err := h.remnawaveClient.DeleteUser(checkCtx, oldID, oldUUID); err != nil {
			slog.Warn("mini app: delete expired reissue", "error", err, "reissueId", item.ID)
		} else if err := h.subscriptionRepository.MarkReissueDeleted(checkCtx, item.ID); err != nil {
			slog.Warn("mini app: mark reissue deleted", "error", err, "reissueId", item.ID)
		}
	}
}
