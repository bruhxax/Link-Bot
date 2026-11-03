package handler

import (
	"context"
	"log/slog"
	"net/url"
	"sort"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

const (
	miniAppDashboardEmojiID = "5278413853577734640"
	miniAppTariffsEmojiID   = "5206626000665868017"
	miniAppSupportEmojiID   = "5206222720416643915"
	miniAppTrialEmojiID     = "5276422526350681413"
)

func (h Handler) hasMiniApp() bool {
	return h.featureEnabled("mini_app") && config.GetMiniAppURL() != ""
}

func (h Handler) miniAppURL(page string) string {
	base := config.GetMiniAppURL()
	if base == "" || page == "" {
		return base
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}

	query := parsed.Query()
	if strings.HasPrefix(page, "custom-page:") {
		query.Set("page", "custom-page")
		query.Set("customPageID", strings.TrimPrefix(page, "custom-page:"))
	} else {
		query.Set("page", page)
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func p2PCheckoutURL(planID string) string {
	parsed, err := url.Parse(config.GetMiniAppURL())
	if err != nil {
		return config.GetMiniAppURL()
	}
	query := parsed.Query()
	query.Set("page", "plans")
	query.Set("planId", planID)
	query.Set("paymentMethod", "p2p")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (h Handler) miniAppKeyboard(lang string) [][]models.InlineKeyboardButton {
	return h.configuredMiniAppKeyboard(context.Background(), nil, lang)
}

func (h Handler) miniAppKeyboardForCustomer(ctx context.Context, customer *database.Customer, lang string) [][]models.InlineKeyboardButton {
	return h.configuredMiniAppKeyboard(ctx, customer, lang)
}

func (h Handler) configuredMiniAppKeyboard(ctx context.Context, customer *database.Customer, lang string) [][]models.InlineKeyboardButton {
	if !h.hasMiniApp() {
		return nil
	}
	menu := h.telegramStartMenuSettings()
	buttons := menu.Buttons
	if buttons == nil {
		buttons = runtimeconfig.DefaultStartButtons(menu)
	}
	needsTrial, needsNotifications := false, false
	for _, item := range buttons {
		needsTrial = needsTrial || item.Kind == "trial"
		needsNotifications = needsNotifications || item.Kind == "notifications"
	}
	trialAllowed := false
	if needsTrial && customer != nil {
		allowed, err := h.canOfferTrial(ctx, customer)
		if err != nil {
			slog.Error("Error checking trial eligibility for start keyboard", "error", err)
		} else {
			trialAllowed = allowed
		}
	}
	notificationsKnown, subscribed := false, false
	if needsNotifications && customer != nil {
		var err error
		subscribed, err = h.customerRepository.BroadcastSubscribed(ctx, customer.ID)
		if err != nil {
			slog.Warn("bot: load Telegram broadcast preference", "error", err)
		} else {
			notificationsKnown = true
		}
	}
	rows := map[int][]models.InlineKeyboardButton{}
	for _, item := range buttons {
		style := runtimeconfig.TelegramButtonSettings{Text: item.Text, IconCustomEmojiID: item.IconCustomEmojiID, Style: item.Style}
		button := telegramButton(style, item.Text, "")
		switch item.Kind {
		case "trial":
			if !trialAllowed {
				continue
			}
			button.CallbackData = CallbackActivateTrial
		case "plans":
			button.CallbackData = CallbackBuy
		case "notifications":
			if !notificationsKnown {
				continue
			}
			button = notificationsMenuButton(menu, subscribed)
		case "page":
			if item.Target == "admin" && (customer == nil || !h.isAdminTelegramID(customer.TelegramID)) {
				continue
			}
			button.WebApp = &models.WebAppInfo{URL: h.miniAppURL(item.Target)}
		case "url":
			button.URL = item.Target
		default:
			continue
		}
		rows[item.Row] = append(rows[item.Row], button)
	}
	order := make([]int, 0, len(rows))
	for row := range rows {
		order = append(order, row)
	}
	sort.Ints(order)
	keyboard := make([][]models.InlineKeyboardButton, 0, len(order))
	for _, row := range order {
		keyboard = append(keyboard, rows[row])
	}
	return keyboard
}

func (h Handler) telegramStartMenuSettings() runtimeconfig.TelegramStartMenuSettings {
	if h.runtimeSettings == nil {
		return runtimeconfig.DefaultSettings().Content.StartMenu
	}
	settings := h.runtimeSettings.Snapshot()
	return runtimeconfig.LocalizeTelegramDefaults(settings.Content, settings.Localization.Language).StartMenu
}

func (h Handler) miniAppTariffsButtonText(lang string) string {
	text := h.translation.GetText(lang, "buy_button")
	text = strings.ReplaceAll(text, "🚀", "")
	return strings.TrimSpace(text)
}

func (h Handler) sendMiniAppEntry(ctx context.Context, b *bot.Bot, chatID int64, langCode string, text string) error {
	msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: h.miniAppKeyboard(langCode),
		},
	})
	if err == nil && msg != nil {
		h.rememberScreenMessage(chatID, msg.ID)
	}

	return err
}
