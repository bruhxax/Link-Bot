package handler

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"link-bot/internal/paymentchannel"
	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
)

func paymentPlanRef(id string) string {
	hash := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%x", hash[:6])
}

func channelCallback(plan planbook.CheckoutPlan, method string, index int) string {
	value := fmt.Sprintf("%s?r=%s&m=%s", CallbackPayment, paymentPlanRef(plan.ID), method)
	if index >= 0 {
		value += "&c=" + strconv.Itoa(index)
	}
	return value
}

func configuredPaymentKeyboard(methods map[string]bool, plan planbook.CheckoutPlan, settings runtimeconfig.Settings, miniApp bool) [][]models.InlineKeyboardButton {
	if len(settings.PaymentMethodVariants) == 0 && len(settings.PaymentMethodText) == 0 {
		return PaymentMethodsKeyboard(methods, plan, settings.Content.Commerce, settings.PaymentMethodOrder, miniApp)
	}
	keyboard := [][]models.InlineKeyboardButton{}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, settings.PaymentMethodOrder...), runtimeconfig.DefaultPaymentMethodOrder()...) {
		base := settings.PaymentMethodBase(id)
		if seen[id] || !methods[id] || base == "" {
			continue
		}
		seen[id] = true
		rows := PaymentMethodsKeyboard(map[string]bool{base: true}, plan, settings.Content.Commerce, []string{base}, miniApp)
		if len(rows) == 0 {
			continue
		}
		button := rows[0][0]
		if name := strings.TrimSpace(settings.PaymentMethodText[id].Name); name != "" {
			button.Text = name
		} else if base == "card" {
			button.Text = "Банковская карта"
		} else if base == "sbp" {
			button.Text = "СБП"
		}
		if base != "p2p" {
			button.CallbackData = channelCallback(plan, id, -1)
		}
		keyboard = append(keyboard, []models.InlineKeyboardButton{button})
	}
	return keyboard
}

func paymentChannelKeyboard(plan planbook.CheckoutPlan, method string, channels []string) [][]models.InlineKeyboardButton {
	rows := [][]models.InlineKeyboardButton{}
	for index, channel := range channels {
		rows = append(rows, []models.InlineKeyboardButton{{Text: paymentchannel.Label(method, channel), CallbackData: channelCallback(plan, method, index)}})
	}
	return rows
}
