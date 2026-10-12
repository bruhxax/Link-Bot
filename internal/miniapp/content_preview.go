package miniapp

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	telegram "link-bot/internal/handler"
	"link-bot/internal/payment"
	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
	"link-bot/utils"
)

type adminContentTestRequest struct {
	Kind    string                        `json:"kind"`
	Content runtimeconfig.ContentSettings `json:"content"`
}

func (h *Handler) handleAdminContentTest(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if !sess.isAdministrator() {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	if h.telegramBot == nil || sess.User.ID <= 0 || (customer != nil && customer.TelegramIDIsSynthetic) {
		h.writeError(w, http.StatusBadRequest, "telegram_unavailable", "Войдите через Telegram для тестовой отправки")
		return
	}
	var req adminContentTestRequest
	if err := h.decodeJSONRequest(w, r, 512<<10, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный тест контента")
		return
	}
	settings := runtimeconfig.DefaultSettings()
	if h.runtimeSettings != nil {
		settings = h.runtimeSettings.Snapshot()
	}
	settings.Content = req.Content
	if err := runtimeconfig.NormalizeAndValidate(&settings); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_content", err.Error())
		return
	}
	if req.Kind == "start" && h.translation != nil {
		if strings.TrimSpace(settings.Content.StartTextRU) == "" {
			settings.Content.StartTextRU = h.translation.GetText("ru", "greeting")
		}
		if strings.TrimSpace(settings.Content.StartImage) == "" {
			settings.Content.StartImage = h.translation.GetText("ru", "image_start_url")
		}
	}
	plans := planbook.All()
	if h.runtimeSettings != nil {
		plans = h.runtimeSettings.CheckoutPlans()
	}
	methods := map[string]bool{}
	if req.Kind == "payment-methods" {
		var err error
		methods, err = h.availablePaymentMethods(r.Context(), customer)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "payment_methods_failed", "Не удалось загрузить способы оплаты")
			return
		}
	}
	text, banner, keyboard, err := buildContentPreview(req.Kind, settings, plans, methods)
	if strings.HasPrefix(banner, "/mini-app/uploads/") {
		name := strings.TrimPrefix(banner, "/mini-app/uploads/")
		if !uploadedMediaNameRegexp.MatchString(name) || h.logoUploadDir == "" {
			err = fmt.Errorf("загруженный баннер недоступен")
		} else {
			banner = filepath.Join(h.logoUploadDir, name)
		}
	}
	if err == nil {
		err = payment.SendContentPreview(r.Context(), h.telegramBot, sess.User.ID, text, banner, keyboard)
	}
	if err != nil {
		slog.Warn("mini app: send content preview", "error", err, "telegramId", utils.MaskHalfInt64(sess.User.ID))
		h.writeError(w, http.StatusBadRequest, "content_preview_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "content_preview_sent"})
}

func previewButton(settings runtimeconfig.TelegramButtonSettings, callback string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: settings.Text, IconCustomEmojiID: settings.IconCustomEmojiID, Style: settings.Style, CallbackData: callback}
}

func previewWebAppButton(settings runtimeconfig.TelegramButtonSettings, page string, fallback string) models.InlineKeyboardButton {
	button := previewButton(settings, fallback)
	if parsed, err := url.Parse(config.GetMiniAppURL()); err == nil && parsed.Host != "" {
		query := parsed.Query()
		query.Set("page", page)
		parsed.RawQuery = query.Encode()
		button.CallbackData = ""
		button.WebApp = &models.WebAppInfo{URL: parsed.String()}
	}
	return button
}

func buildContentPreview(kind string, settings runtimeconfig.Settings, plans []planbook.CheckoutPlan, methods map[string]bool) (string, string, [][]models.InlineKeyboardButton, error) {
	content := settings.Content
	commerce := content.Commerce
	back := []models.InlineKeyboardButton{previewButton(commerce.BackButton, telegram.CallbackStart)}
	keyboard := [][]models.InlineKeyboardButton{}
	switch kind {
	case "start":
		menu := content.StartMenu
		if settings.Features["trials"] && settings.Trial.Enabled {
			keyboard = append(keyboard, []models.InlineKeyboardButton{previewButton(menu.TrialButton, telegram.CallbackActivateTrial)})
		}
		keyboard = append(keyboard, []models.InlineKeyboardButton{previewWebAppButton(menu.DashboardButton, "dashboard", telegram.CallbackStart)}, []models.InlineKeyboardButton{previewButton(menu.PlansButton, telegram.CallbackBuy)}, []models.InlineKeyboardButton{previewWebAppButton(menu.SupportButton, "support", telegram.CallbackStart)})
		return content.StartTextRU, content.StartImage, keyboard, nil
	case "verification":
		channel := previewButton(content.Verification.ChannelButton, telegram.CallbackStart)
		if strings.TrimSpace(content.Links["channel"]) != "" {
			channel.URL = content.Links["channel"]
			channel.CallbackData = ""
		}
		keyboard = [][]models.InlineKeyboardButton{{channel}, {previewButton(content.Verification.ConfirmButton, telegram.CallbackVerifyChannel)}}
		return content.Verification.Text, content.Verification.Banner, keyboard, nil
	case "verification-failed":
		return content.Verification.CheckFailedText, "", nil, nil
	case "verification-missing":
		return content.Verification.NotSubscribedText, "", nil, nil
	case "verification-success":
		return content.Verification.VerifiedText, "", nil, nil
	case "support-new", "support-customer-reply", "support-admin-reply", "support-closed":
		templates := map[string]string{"support-new": content.Support.NewTicketText, "support-customer-reply": content.Support.CustomerReplyText, "support-admin-reply": content.Support.AdminReplyText, "support-closed": content.Support.ClosedText}
		message := "Тестовое сообщение для проверки оформления"
		if kind == "support-closed" {
			message = ""
		}
		ticket := &database.SupportTicket{ID: 123, Subject: "Тестовое обращение", CustomerName: "Тестовый пользователь", CustomerUsername: "test_user", SubscriptionLabel: "Активна"}
		keyboard = [][]models.InlineKeyboardButton{{previewWebAppButton(content.Support.OpenButton, "support", telegram.CallbackStart)}}
		return renderSupportNotification(templates[kind], ticket, message), "", keyboard, nil
	case "device-added", "device-limit", "review-created":
		keys := map[string]string{"device-added": "deviceAddedTemplate", "device-limit": "deviceLimitReachedTemplate", "review-created": "reviewCreatedTemplate"}
		template := content.Copy["ru"][keys[kind]]
		if template == "" {
			template = runtimeconfig.DefaultSettings().Content.Copy["ru"][keys[kind]]
		}
		text := strings.NewReplacer("{added}", "1", "{count}", "3", "{limit}", "3", "{name}", "Тестовый пользователь", "{username}", "@test_user", "{rating}", "5", "{comment}", "Тестовый отзыв для проверки оформления").Replace(template)
		keyboard = [][]models.InlineKeyboardButton{{previewWebAppButton(content.Support.OpenButton, "dashboard", telegram.CallbackStart)}}
		return text, "", keyboard, nil
	case "tariffs":
		for _, plan := range plans {
			label := plan.Name
			if label == "" {
				label = fmt.Sprintf("%d мес.", plan.Months)
				if plan.Days > 0 {
					label = fmt.Sprintf("%d дн.", plan.Days)
				}
			}
			keyboard = append(keyboard, []models.InlineKeyboardButton{{Text: fmt.Sprintf("%s | %d ₽", label, plan.PriceRub), CallbackData: fmt.Sprintf("%s?planId=%s", telegram.CallbackSell, plan.ID)}})
		}
		keyboard = append(keyboard, back)
		return commerce.TariffsText, commerce.Banner, keyboard, nil
	case "payment-methods":
		for _, plan := range plans {
			if plan.PriceRub > 0 || plan.PriceStars > 0 {
				keyboard = telegram.PaymentMethodsKeyboard(methods, plan, commerce, settings.PaymentMethodOrder, settings.Features["mini_app"] && config.GetMiniAppURL() != "")
				break
			}
		}
		// Preview clicks return to tariff selection and never charge the sample plan.
		for row := range keyboard {
			for col := range keyboard[row] {
				if keyboard[row][col].CallbackData != "" {
					keyboard[row][col].CallbackData = telegram.CallbackBuy
				}
			}
		}
		keyboard = append(keyboard, back)
		return commerce.PaymentMethodsText, commerce.Banner, keyboard, nil
	case "payment-ready":
		keyboard = [][]models.InlineKeyboardButton{{previewWebAppButton(commerce.PayButton, "plans", telegram.CallbackBuy)}, back}
		return commerce.PaymentReadyText, commerce.Banner, keyboard, nil
	default:
		return "", "", nil, fmt.Errorf("неизвестный экран: %s", kind)
	}
}
