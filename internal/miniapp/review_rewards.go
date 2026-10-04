package miniapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
)

func (h *Handler) reviewRewards() runtimeconfig.ReviewRewardSettings {
	if h.runtimeSettings != nil {
		return h.runtimeSettings.Snapshot().ReviewRewards
	}
	return runtimeconfig.DefaultReviewRewards()
}

func reviewRewardSuccessMessage(snapshot []byte) string {
	var reward runtimeconfig.ReviewRewardSettings
	if json.Unmarshal(snapshot, &reward) == nil && reward.Days == 0 && reward.TrafficGB == 0 && reward.BalanceRub == 0 && !reward.Promo.Enabled {
		return "Спасибо за отзыв!"
	}
	return "Отзыв сохранён. Вознаграждение выдано"
}

func (h *Handler) handleAdminReviewRewards(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Доступ запрещён")
		return
	}
	if h.runtimeSettings == nil {
		h.writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "Настройки недоступны")
		return
	}
	var reward runtimeconfig.ReviewRewardSettings
	if err := h.decodeJSONRequest(w, r, 4096, &reward); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if err := reward.Validate(); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_reward", err.Error())
		return
	}
	settings, err := h.runtimeSettings.UpdateReviewRewards(r.Context(), reward, sess.User.ID)
	if err != nil {
		slog.Error("save review rewards", "error", err)
		h.writeError(w, http.StatusInternalServerError, "save_failed", "Не удалось сохранить настройки")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": settings.ReviewRewards})
}

func (h *Handler) handleRetryReviewReward(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if h.reviewRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "reviews_unavailable", "Отзывы недоступны")
		return
	}
	review, err := h.reviewRepository.FindAnyByCustomerID(r.Context(), customer.ID)
	if err != nil || review == nil {
		h.writeError(w, http.StatusNotFound, "review_not_found", "Отзыв не найден")
		return
	}
	if err := h.grantReviewReward(contextWithSessionTelegramProfile(r.Context(), sess), customer, review.ID); err != nil {
		slog.Error("retry review reward", "error", err, "reviewId", review.ID)
		h.writeError(w, http.StatusBadGateway, "review_reward_failed", "Подарок пока не выдан. Для награды трафиком нужна подписка; повторите позже")
		return
	}
	payload, err := h.buildBootstrapResponse(r.Context(), sess, customer)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "refresh_failed", "Награда выдана, обновите страницу")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Вознаграждение выдано", "data": payload})
}

// Persist absolute targets before calling the panel. Retrying never adds the
// same days/traffic again, and a later purchase is never reduced.
func reviewAccessTarget(state *remnawave.UserState, reward runtimeconfig.ReviewRewardSettings, now time.Time) (*time.Time, *int64, error) {
	var expires *time.Time
	var traffic *int64
	if reward.Days > 0 {
		base := now
		if state != nil && state.ExpireAt != nil && state.ExpireAt.After(base) {
			base = *state.ExpireAt
		}
		target := base.Add(time.Duration(reward.Days) * 24 * time.Hour)
		expires = &target
	}
	if reward.TrafficGB > 0 {
		if state == nil || !state.Exists {
			if reward.Days == 0 {
				return nil, nil, errors.New("traffic reward requires a subscription")
			}
			target := int64(reward.TrafficGB) * adminUserTrafficGB
			traffic = &target
		} else if state.TrafficLimitBytes > 0 {
			if state.TrafficLimitBytes > math.MaxInt64-int64(reward.TrafficGB)*adminUserTrafficGB {
				return nil, nil, errors.New("traffic limit is too large")
			}
			target := state.TrafficLimitBytes + int64(reward.TrafficGB)*adminUserTrafficGB
			traffic = &target
		}
	}
	return expires, traffic, nil
}

func (h *Handler) grantReviewReward(ctx context.Context, customer *database.Customer, reviewID int64) error {
	if h.reviewRepository == nil || customer == nil {
		return errors.New("review rewards unavailable")
	}
	unlock, err := h.reviewRepository.AcquireRewardLock(ctx, customer.ID)
	if err != nil {
		return err
	}
	defer unlock()
	review, err := h.reviewRepository.FindAnyByCustomerID(ctx, customer.ID)
	if err != nil {
		return err
	}
	if review == nil || review.ID != reviewID {
		return errors.New("review not found")
	}
	if review.RewardGranted {
		return nil
	}
	var reward runtimeconfig.ReviewRewardSettings
	if err := json.Unmarshal(review.RewardSettings, &reward); err != nil {
		return err
	}
	if err := reward.Validate(); err != nil {
		return err
	}
	// These components have their own durable idempotency keys and do not
	// depend on the availability of the subscription panel.
	if reward.BalanceRub > 0 {
		if h.walletRepository == nil {
			return errors.New("wallet unavailable")
		}
		if _, _, err := h.walletRepository.Apply(ctx, customer.ID, int64(reward.BalanceRub)*100, "review_reward", fmt.Sprintf("review-reward:%d", reviewID), "Вознаграждение за отзыв"); err != nil {
			return err
		}
	}
	if reward.Promo.Enabled {
		if h.promoCodeRepository == nil {
			return errors.New("promos unavailable")
		}
		p := &database.PromoCode{RewardType: reward.Promo.RewardType, RewardValue: reward.Promo.RewardValue, RewardTrafficGB: reward.Promo.RewardTrafficGB, DiscountPercent: reward.Promo.DiscountPercent}
		if reward.Promo.ExpiryDays > 0 {
			expiry := time.Now().UTC().Add(time.Duration(reward.Promo.ExpiryDays) * 24 * time.Hour)
			p.ExpiresAt = &expiry
		}
		if _, err := h.promoCodeRepository.IssueReviewPromo(ctx, reviewID, customer.ID, p); err != nil {
			return err
		}
	}
	if reward.Days > 0 || reward.TrafficGB > 0 {
		if h.remnawaveClient == nil {
			return errors.New("panel unavailable")
		}
		claim, err := h.reviewRepository.RewardClaim(ctx, reviewID)
		if err != nil {
			return err
		}
		if !claim.AccessApplied {
			if !claim.TargetInitialized {
				active, _, err := h.loadCustomerSubscriptions(ctx, customer)
				if err != nil {
					return err
				}
				state, err := h.panelStateForCustomerSubscription(ctx, customer, active)
				if err != nil {
					return err
				}
				claim.TargetExpiresAt, claim.TargetTrafficBytes, err = reviewAccessTarget(state, reward, time.Now().UTC())
				if err != nil {
					return err
				}
				if active != nil && active.ID > 0 {
					claim.SubscriptionID = &active.ID
				}
				if state != nil && state.Exists {
					claim.PanelUserID = state.UserID
					if state.UserUUID != uuid.Nil {
						claim.PanelUserUUID = &state.UserUUID
					}
				}
				if err := h.reviewRepository.SaveRewardTarget(ctx, claim); err != nil {
					return err
				}
			}
			userUUID := uuid.Nil
			if claim.PanelUserUUID != nil {
				userUUID = *claim.PanelUserUUID
			}
			var updated *remnawave.PanelUser
			if claim.PanelUserID == 0 && userUUID == uuid.Nil {
				traffic := int64(0)
				if claim.TargetTrafficBytes != nil {
					traffic = *claim.TargetTrafficBytes
				}
				var subscription *database.CustomerSubscription
				if claim.SubscriptionID != nil && h.subscriptionRepository != nil {
					subscription, err = h.subscriptionRepository.FindForCustomer(ctx, customer.ID, *claim.SubscriptionID)
					if err != nil {
						return err
					}
					if subscription == nil {
						return errors.New("reward subscription was deleted")
					}
				}
				if subscription != nil && !subscription.IsPrimary {
					updated, err = h.remnawaveClient.EnsureReviewSecondaryAccessTarget(ctx, customer.ID, customer.TelegramID, subscription.ID, traffic, config.DeviceLimitForMonths(1), claim.TargetExpiresAt)
				} else {
					updated, err = h.remnawaveClient.EnsureReviewAccessTarget(ctx, customer.ID, customer.TelegramID, traffic, config.DeviceLimitForMonths(1), claim.TargetExpiresAt)
				}
			} else {
				updated, err = h.remnawaveClient.ApplyUserAccessTarget(ctx, claim.PanelUserID, userUUID, claim.TargetExpiresAt, claim.TargetTrafficBytes)
			}
			if err != nil {
				return err
			}
			if claim.SubscriptionID != nil && h.subscriptionRepository != nil {
				sub, err := h.subscriptionRepository.FindForCustomer(ctx, customer.ID, *claim.SubscriptionID)
				if err != nil {
					return err
				}
				if sub == nil {
					return errors.New("reward subscription was deleted")
				}
				if err := h.persistAdminSubscriptionPanelState(ctx, sub, updated); err != nil {
					return err
				}
			} else {
				if err := h.customerRepository.UpdateFields(ctx, customer.ID, map[string]any{"subscription_link": updated.SubscriptionURL, "expire_at": updated.ExpireAt}); err != nil {
					return err
				}
				customer.SubscriptionLink = &updated.SubscriptionURL
				customer.ExpireAt = &updated.ExpireAt
			}
			if err := h.reviewRepository.MarkAccessApplied(ctx, reviewID); err != nil {
				return err
			}
		}
	}
	return h.reviewRepository.MarkRewardGranted(ctx, reviewID, reward.Days, int64(reward.TrafficGB)*adminUserTrafficGB)
}
