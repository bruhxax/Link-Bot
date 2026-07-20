package handler

import (
	"context"
	"fmt"
	"log/slog"
	"link-bot/internal/config"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) ReferralCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, _ := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	langCode := update.CallbackQuery.From.LanguageCode
	refCode := customer.TelegramID

	refLink := fmt.Sprintf(
		"https://telegram.me/share/url?url=https://t.me/%s?start=ref_%d",
		update.CallbackQuery.Message.Message.From.Username,
		refCode,
	)

	count, err := h.referralRepository.CountGrantedByReferrer(ctx, customer.TelegramID)
	if err != nil {
		slog.Error("error counting referrals", "error", err)
		return
	}

	text := buildReferralScreenText(langCode, count)

	keyboard := [][]models.InlineKeyboardButton{
		{{Text: h.translation.GetText(langCode, "share_referral_button"), URL: refLink}},
		{h.premiumBackButton(CallbackStart)},
	}

	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		slog.Error("ReferralCallbackHandler: callback message is nil")
		return
	}

	// ✅ Обновляем одно меню-сообщение (картинка + caption + кнопки), без удаления сообщений
	h.editScreen(ctx, b,
		msg.Chat.ID,
		msg.ID,
		h.translation.GetText(langCode, "image_referral_url"),
		text,
		keyboard,
	)
}

func buildReferralScreenText(langCode string, count int) string {
	days := config.GetReferralDays()
	trafficGb := config.ReferralTrafficBonusBytes() / (1024 * 1024 * 1024)

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(langCode)), "en") {
		return fmt.Sprintf(
			"<b>Referral program</b>\n\nSuccessful referrals: %d\nReward for each paid friend: +%d days and +%d GB\n\nThe bonus is credited only after the invited user purchases any subscription.",
			count,
			days,
			trafficGb,
		)
	}

	return fmt.Sprintf(
		"<tg-emoji emoji-id='5258513401784573443'>☺️</tg-emoji> <b>Реферальная программа</b>\n\nУспешных приглашений: %d\nБонус за каждого оплаченного друга: +%d дней и +%d ГБ\n\nБонус начисляется только после того, как приглашённый пользователь оплатит любой тариф.",
		count,
		days,
		trafficGb,
	)
}
