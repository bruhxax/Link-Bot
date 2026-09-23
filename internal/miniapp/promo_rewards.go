package miniapp

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/utils"
)

var (
	errPromoSubscriptionUnavailable = errors.New("subscription is unavailable for this reward")
	errPromoTrafficUnlimited        = errors.New("unlimited subscription traffic cannot be increased")
)

func promoAccessTarget(state *remnawave.UserState, promo *database.PromoCode, now time.Time) (*time.Time, *int64, error) {
	if state == nil || !state.Exists || promo == nil {
		return nil, nil, errPromoSubscriptionUnavailable
	}
	switch promo.RewardType {
	case "days", "days_traffic":
		base := now.UTC()
		if state.ExpireAt != nil && state.ExpireAt.After(base) {
			base = state.ExpireAt.UTC()
		}
		target := base.AddDate(0, 0, promo.RewardValue)
		if promo.RewardType == "days" {
			return &target, nil, nil
		}
		trafficTarget, err := promoTrafficTarget(state.TrafficLimitBytes, promo.RewardTrafficGB)
		if err != nil {
			return nil, nil, err
		}
		return &target, &trafficTarget, nil
	case "traffic":
		if !state.Active {
			return nil, nil, errPromoSubscriptionUnavailable
		}
		target, err := promoTrafficTarget(state.TrafficLimitBytes, promo.RewardValue)
		if err != nil {
			return nil, nil, err
		}
		return nil, &target, nil
	default:
		return nil, nil, database.ErrPromoCodeInvalidReward
	}
}

func promoTrafficTarget(current int64, gigabytes int) (int64, error) {
	if current <= 0 {
		return 0, errPromoTrafficUnlimited
	}
	add := int64(gigabytes) * 1024 * 1024 * 1024
	if gigabytes <= 0 || add > math.MaxInt64-current {
		return 0, errPromoSubscriptionUnavailable
	}
	return current + add, nil
}

func (h *Handler) handleRedeemPromoCode(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	var req promoCodeApplyRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	promo, code, errCode, err := h.resolvePromoCode(r.Context(), customer.ID, req.Code)
	if err != nil {
		slog.Error("mini app: resolve promo reward", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusInternalServerError, "promo_failed", promoErrorMessage("promo_failed"))
		return
	}
	if errCode != "" {
		h.writeError(w, http.StatusBadRequest, errCode, promoErrorMessage(errCode))
		return
	}
	if promo.RewardType == "discount" {
		h.writeError(w, http.StatusBadRequest, "promo_discount_checkout_only", promoErrorMessage("promo_discount_checkout_only"))
		return
	}
	if customer.IsBlocked {
		h.writeError(w, http.StatusForbidden, "forbidden", "Account is blocked")
		return
	}
	if promo.RewardType != "balance" {
		unlock, lockErr := h.promoCodeRepository.AcquireSubscriptionRewardLock(r.Context(), customer.ID)
		if lockErr != nil {
			slog.Error("mini app: lock subscription promo reward", "error", lockErr, "customerId", utils.MaskHalfInt64(customer.ID))
			h.writeError(w, http.StatusServiceUnavailable, "promo_reward_failed", promoErrorMessage("promo_reward_failed"))
			return
		}
		defer unlock()
	}

	existing, err := h.promoCodeRepository.FindRewardRedemption(r.Context(), promo.ID, customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "promo_failed", promoErrorMessage("promo_failed"))
		return
	}
	var subscriptionID *int64
	var targetExpiresAt *time.Time
	var targetTrafficBytes *int64
	if existing == nil {
		if promo.RewardType == "balance" {
			if h.walletRepository == nil {
				h.writeError(w, http.StatusServiceUnavailable, "promo_wallet_unavailable", promoErrorMessage("promo_wallet_unavailable"))
				return
			}
		} else {
			active, _, loadErr := h.loadCustomerSubscriptions(r.Context(), customer)
			if loadErr != nil || active == nil {
				h.writeError(w, http.StatusConflict, "promo_subscription_unavailable", promoErrorMessage("promo_subscription_unavailable"))
				return
			}
			target, _, targetErr := h.adminUserSubscriptionTarget(r.Context(), customer.ID, active.ID)
			if targetErr != nil {
				h.writeError(w, http.StatusConflict, "promo_subscription_unavailable", promoErrorMessage("promo_subscription_unavailable"))
				return
			}
			targetExpiresAt, targetTrafficBytes, targetErr = promoAccessTarget(target.state, promo, time.Now().UTC())
			if targetErr != nil {
				code := "promo_subscription_unavailable"
				if errors.Is(targetErr, errPromoTrafficUnlimited) {
					code = "promo_traffic_unlimited"
				}
				h.writeError(w, http.StatusConflict, code, promoErrorMessage(code))
				return
			}
			subscriptionID = &active.ID
		}
	}

	claim, err := h.promoCodeRepository.ClaimReward(r.Context(), promo, customer.ID, subscriptionID, targetExpiresAt, targetTrafficBytes)
	if err != nil {
		code := "promo_failed"
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, database.ErrPromoCodeAlreadyUsed):
			code, status = "promo_already_used", http.StatusConflict
		case errors.Is(err, database.ErrPromoCodeLimitReached):
			code, status = "promo_limit_reached", http.StatusConflict
		case errors.Is(err, database.ErrPromoCodeUnavailable):
			code, status = "promo_inactive", http.StatusConflict
		default:
			slog.Error("mini app: claim promo reward", "error", err, "promoId", promo.ID)
		}
		h.writeError(w, status, code, promoErrorMessage(code))
		return
	}

	if promo.RewardType == "balance" {
		if h.walletRepository == nil {
			h.writeError(w, http.StatusServiceUnavailable, "promo_wallet_unavailable", promoErrorMessage("promo_wallet_unavailable"))
			return
		}
		_, _, err = h.walletRepository.Apply(r.Context(), customer.ID, int64(promo.RewardValue)*100, "promo_reward", fmt.Sprintf("promo-reward:%d", claim.ID), "Промокод "+code)
	} else {
		if claim.SubscriptionID == nil {
			err = errPromoSubscriptionUnavailable
		} else {
			var subscription *database.CustomerSubscription
			var target *adminSubscriptionTarget
			target, subscription, err = h.adminUserSubscriptionTarget(r.Context(), customer.ID, *claim.SubscriptionID)
			if err == nil {
				var updated *remnawave.PanelUser
				updated, err = h.remnawaveClient.ApplyUserAccessTarget(r.Context(), target.state.UserID, target.state.UserUUID, claim.TargetExpiresAt, claim.TargetTrafficBytes)
				if err == nil {
					err = h.persistAdminSubscriptionPanelState(r.Context(), subscription, updated)
				}
			}
		}
	}
	if err != nil {
		messageCode := "promo_reward_failed"
		if strings.Contains(err.Error(), "unlimited traffic") {
			messageCode = "promo_traffic_unlimited"
		}
		slog.Error("mini app: apply promo reward", "error", err, "promoId", promo.ID, "customerId", utils.MaskHalfInt64(customer.ID))
		h.writeError(w, http.StatusBadGateway, messageCode, promoErrorMessage(messageCode))
		return
	}
	if err := h.promoCodeRepository.MarkRewardApplied(r.Context(), claim.ID); err != nil {
		slog.Error("mini app: finish promo reward", "error", err, "promoId", promo.ID, "customerId", utils.MaskHalfInt64(customer.ID))
		h.writeError(w, http.StatusInternalServerError, "promo_reward_failed", promoErrorMessage("promo_reward_failed"))
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": promoCodePayload{
		ID: promo.ID, Code: code, RewardType: promo.RewardType, RewardValue: promo.RewardValue, RewardTrafficGB: promo.RewardTrafficGB,
	}})
}
