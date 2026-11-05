package payment

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

func InlineGiftBannerURL(configured, miniAppURL string) string {
	configured = strings.TrimSpace(configured)
	if strings.HasPrefix(configured, "https://") {
		return configured
	}
	if !strings.HasPrefix(configured, "/mini-app/uploads/") {
		return ""
	}
	base, err := url.Parse(miniAppURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return ""
	}
	return base.ResolveReference(&url.URL{Path: configured}).String()
}

func InlineGiftMessagePreview(configured, miniAppURL string) *models.LinkPreviewOptions {
	url := InlineGiftBannerURL(configured, miniAppURL)
	if url == "" {
		return nil
	}
	yes := true
	return &models.LinkPreviewOptions{URL: &url, PreferLargeMedia: &yes, ShowAboveText: &yes}
}

func (s PaymentService) publishInlineGift(ctx context.Context, purchase *database.Purchase, sender *database.Customer) error {
	if s.telegramBot == nil || s.purchaseRepository == nil || purchase == nil || purchase.GiftToken == nil || !isInlineGift(purchase) {
		return errors.New("inline gift delivery is unavailable")
	}
	draft, err := s.purchaseRepository.FindInlineGift(ctx, *purchase.GiftToken)
	if err != nil || draft == nil {
		if err == nil {
			err = errors.New("inline gift draft not found")
		}
		return err
	}
	if sender == nil {
		sender, err = s.customerRepository.FindById(ctx, purchase.CustomerID)
		if err != nil {
			return err
		}
	}
	if sender == nil || sender.TelegramID != draft.SenderTelegramID || draft.PlanID != pointerValue(purchase.PlanID) {
		return errors.New("inline gift purchase does not match its draft")
	}
	settings := s.giftSettings().Inline
	plan := s.giftPlanLabel(purchase)
	if s.runtimeSettings != nil && purchase.PlanID != nil {
		if localized := s.runtimeSettings.PlanTitle(*purchase.PlanID, sender.Language); localized != "" {
			plan = localized
		}
	}
	replacements := s.giftReplacements(purchase, sender, nil)
	replacements["{plan}"] = plan
	replacements["{recipient}"] = "получателю"
	caption := renderGiftTemplate(settings.Text, replacements)
	buttonText := strings.TrimSpace(settings.ButtonText)
	if buttonText == "" {
		buttonText = "Получить"
	}
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: buttonText, URL: GiftShareURL(purchase)},
	}}}
	if draft.HasBanner {
		_, err = s.telegramBot.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
			InlineMessageID: draft.InlineMessageID, Caption: caption,
			ParseMode: models.ParseModeHTML, ReplyMarkup: keyboard,
		})
	} else {
		_, err = s.telegramBot.EditMessageText(ctx, &bot.EditMessageTextParams{
			InlineMessageID: draft.InlineMessageID, Text: caption,
			ParseMode: models.ParseModeHTML, ReplyMarkup: keyboard,
			LinkPreviewOptions: InlineGiftMessagePreview(settings.BannerURL, config.GetMiniAppURL()),
		})
	}
	if err != nil {
		return err
	}
	return s.purchaseRepository.MarkInlineGiftPublished(ctx, draft.ID)
}

func (s PaymentService) RunInlineGiftPublisher(ctx context.Context) {
	if s.purchaseRepository == nil || s.telegramBot == nil {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		items, err := s.purchaseRepository.ListUnpublishedPaidInlineGifts(ctx, 25)
		if err != nil && ctx.Err() == nil {
			slog.Error("payment: list pending inline gifts failed", "error", err)
		}
		for _, item := range items {
			purchase, findErr := s.purchaseRepository.FindById(ctx, *item.PurchaseID)
			if findErr == nil && purchase != nil {
				findErr = s.publishInlineGift(ctx, purchase, nil)
			}
			if findErr != nil && ctx.Err() == nil {
				slog.Warn("payment: retry inline gift failed", "error", findErr)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s PaymentService) clearInlineGiftClaimButton(ctx context.Context, purchase *database.Purchase) {
	if s.telegramBot == nil || purchase == nil || purchase.GiftToken == nil {
		return
	}
	draft, err := s.purchaseRepository.FindInlineGift(ctx, *purchase.GiftToken)
	if err != nil || draft == nil {
		return
	}
	_, err = s.telegramBot.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		InlineMessageID: draft.InlineMessageID,
		ReplyMarkup:     models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
	})
	if err != nil {
		slog.Warn("payment: clear claimed inline gift button failed", "error", err)
	}
}
