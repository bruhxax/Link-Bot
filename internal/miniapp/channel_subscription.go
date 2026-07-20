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
	if !config.IsRequiredChannelSubscriptionEnabled() || customer == nil {
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

	chatID, ok := config.RequiredChannelSubscriptionChatID()
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
		ChannelURL:   config.RequiredChannelSubscriptionURL(),
		ChannelTitle: h.requiredChannelSubscriptionTitle(),
		ImageURL:     "",
	}
}

func (h *Handler) requiredChannelSubscriptionTitle() string {
	if title := strings.TrimSpace(config.RequiredChannelSubscriptionTitle()); title != "" {
		return title
	}

	raw := strings.TrimSpace(config.RequiredChannelSubscriptionURL())
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
