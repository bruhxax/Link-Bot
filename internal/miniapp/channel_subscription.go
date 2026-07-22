package miniapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

type channelSubscriptionMeta struct {
	ChannelURL   string `json:"channelUrl"`
	ChannelTitle string `json:"channelTitle"`
	ImageURL     string `json:"imageUrl"`
}

const (
	channelSubscriptionStatusSubscribed   = 1
	channelSubscriptionStatusUnsubscribed = -1
)

func (h *Handler) maybeAutoVerifyRequiredChannelSubscription(ctx context.Context, customer *database.Customer) (bool, error) {
	return h.verifyRequiredChannelSubscription(ctx, customer, false)
}

func (h *Handler) verifyRequiredChannelSubscription(ctx context.Context, customer *database.Customer, force bool) (bool, error) {
	if h.requiredChannelSubscriptionURL() == "" || customer == nil {
		return true, nil
	}
	if h.shouldBypassRequiredChannelSubscription(customer.TelegramID) {
		return true, nil
	}
	if !force && customer.ChannelSubscriptionVerifiedAt != nil && time.Since(*customer.ChannelSubscriptionVerifiedAt) < 30*time.Minute {
		if h.channelSubCache != nil {
			h.channelSubCache.Set(customer.TelegramID, channelSubscriptionStatusSubscribed)
		}
		return true, nil
	}

	if !force && h.channelSubCache != nil {
		if status, ok := h.channelSubCache.Get(customer.TelegramID); ok {
			return status == channelSubscriptionStatusSubscribed, nil
		}
	}

	subscribed, err := h.isUserSubscribedToRequiredChannel(ctx, customer.TelegramID)
	if err != nil {
		return false, err
	}

	if h.channelSubCache != nil {
		if subscribed {
			h.channelSubCache.Set(customer.TelegramID, channelSubscriptionStatusSubscribed)
		} else {
			h.channelSubCache.Set(customer.TelegramID, channelSubscriptionStatusUnsubscribed)
		}
	}

	if !subscribed {
		if customer.ChannelSubscriptionVerifiedAt != nil {
			if err := h.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
				"channel_subscription_verified_at": nil,
			}); err != nil {
				return false, err
			}
			customer.ChannelSubscriptionVerifiedAt = nil
		}
		return false, nil
	}

	if customer.ChannelSubscriptionVerifiedAt == nil {
		verifiedAt := time.Now().UTC()
		if err := h.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"channel_subscription_verified_at": verifiedAt,
		}); err != nil {
			return false, err
		}
		customer.ChannelSubscriptionVerifiedAt = &verifiedAt
	}
	return true, nil
}

func (h *Handler) shouldBypassRequiredChannelSubscription(telegramID int64) bool {
	return telegramID == config.GetAdminTelegramId() || config.GetWhitelistedTelegramIds()[telegramID]
}

func (h *Handler) isUserSubscribedToRequiredChannel(ctx context.Context, telegramID int64) (bool, error) {
	if h.telegramBot == nil {
		return false, fmt.Errorf("telegram bot is nil")
	}

	chatID, ok := h.requiredChannelSubscriptionChatID()
	if !ok {
		return false, fmt.Errorf("required channel subscription chat id is not configured")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	member, err := h.telegramBot.GetChatMember(checkCtx, &bot.GetChatMemberParams{
		ChatID: chatID,
		UserID: telegramID,
	})
	if err != nil {
		return false, err
	}

	switch member.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember:
		return true, nil
	case models.ChatMemberTypeRestricted:
		return member.Restricted != nil && member.Restricted.IsMember, nil
	default:
		return false, nil
	}
}

func (h *Handler) requiredChannelSubscriptionMeta() channelSubscriptionMeta {
	return channelSubscriptionMeta{
		ChannelURL:   h.requiredChannelSubscriptionURL(),
		ChannelTitle: h.requiredChannelSubscriptionTitle(),
		ImageURL:     "",
	}
}

func (h *Handler) requiredChannelSubscriptionTitle() string {
	if h.runtimeSettings != nil {
		if title := strings.TrimSpace(h.runtimeSettings.Snapshot().Content.Verification.ChannelButton.Text); title != "" {
			return title
		}
	} else if title := strings.TrimSpace(config.RequiredChannelSubscriptionTitle()); title != "" {
		return title
	}

	raw := h.requiredChannelSubscriptionURL()
	raw = strings.TrimPrefix(raw, "https://t.me/")
	raw = strings.TrimPrefix(raw, "http://t.me/")
	raw = strings.TrimPrefix(raw, "t.me/")
	raw = strings.TrimPrefix(raw, "@")
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "Channel"
	}

	return raw
}

func (h *Handler) requiredChannelSubscriptionURL() string {
	if h.runtimeSettings != nil {
		return strings.TrimSpace(h.runtimeSettings.Snapshot().Content.Links["channel"])
	}
	return strings.TrimSpace(config.RequiredChannelSubscriptionURL())
}

func (h *Handler) requiredChannelSubscriptionChatID() (any, bool) {
	raw := h.requiredChannelSubscriptionURL()
	if h.runtimeSettings != nil {
		if configured := strings.TrimSpace(h.runtimeSettings.Snapshot().Content.Verification.ChannelChatID); configured != "" {
			raw = configured
		}
	}
	return runtimeconfig.ParseTelegramChannelChatID(raw)
}
