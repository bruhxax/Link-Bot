package miniapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"link-bot/internal/database"
)

// Inline actions use the same permission checks and subscription target as the
// administrator panel. The target date is fixed when the inline result is
// created, so a repeated Telegram callback cannot add the same days twice.
func (h *Handler) ExtendInlineUser(ctx context.Context, actorID, customerID, subscriptionID int64, target time.Time) error {
	if !h.HasAdminPermission(ctx, actorID, "users.subscription") {
		return errors.New("Нет права изменять подписки")
	}
	if customerID <= 0 || subscriptionID <= 0 || !target.After(time.Now().UTC()) || target.After(time.Now().UTC().AddDate(11, 0, 0)) {
		return errors.New("Некорректный срок подписки")
	}
	user, subscription, err := h.adminUserSubscriptionTarget(ctx, customerID, subscriptionID)
	if err != nil {
		return err
	}
	if user.customer.IsBlocked {
		return errors.New("Пользователь заблокирован")
	}
	return h.runInlineAdminAction(ctx, actorID, user.customer, "users/subscription", "Продлил подписку через inline",
		[]database.AdminActivityDetail{{Label: "Подписка", Value: fmt.Sprint(subscriptionID)}, {Label: "Новый срок", Value: target.UTC().Format(time.RFC3339)}}, func() error {
			updated, err := h.remnawaveClient.ApplyUserAccessTarget(ctx, user.state.UserID, user.state.UserUUID, &target, nil)
			if err != nil {
				return err
			}
			return h.persistAdminSubscriptionPanelState(ctx, subscription, updated)
		})
}

func (h *Handler) CreditInlineUser(ctx context.Context, actorID, customerID, rub int64, reference string) error {
	if !h.HasAdminPermission(ctx, actorID, "users.balance") {
		return errors.New("Нет права изменять баланс")
	}
	if customerID <= 0 || rub < 1 || rub > 1000000 || len(reference) < 8 || len(reference) > 48 {
		return errors.New("Некорректная сумма")
	}
	if h.walletRepository == nil {
		return errors.New("Баланс временно недоступен")
	}
	customer, err := h.customerRepository.FindById(ctx, customerID)
	if err != nil || customer == nil {
		return errors.New("Пользователь не найден")
	}
	return h.runInlineAdminAction(ctx, actorID, customer, "users/balance", "Пополнил баланс через inline",
		[]database.AdminActivityDetail{{Label: "Сумма", Value: fmt.Sprintf("%d ₽", rub)}}, func() error {
			_, _, err := h.walletRepository.Apply(ctx, customerID, rub*100, "admin_credit", "inline_admin:"+reference, "Пополнение администратором через inline")
			return err
		})
}

func (h *Handler) runInlineAdminAction(ctx context.Context, actorID int64, customer *database.Customer, action, title string, details []database.AdminActivityDetail, apply func() error) error {
	repo := h.activityRepository()
	if repo == nil {
		return errors.New("Журнал действий недоступен")
	}
	name := fmt.Sprint(customer.TelegramID)
	if customer.TelegramUsername != nil && *customer.TelegramUsername != "" {
		name = "@" + *customer.TelegramUsername
	}
	entry := database.AdminActivity{ActorTelegramID: actorID, Action: action, Category: "users", Title: title,
		TargetCustomerID: customer.ID, TargetTelegramID: customer.TelegramID, TargetName: name, Details: details, Status: "pending"}
	id, err := repo.CreateAdminActivity(ctx, entry)
	if err != nil {
		return fmt.Errorf("Не удалось записать действие: %w", err)
	}
	entry.ID = id
	err = apply()
	entry.Status = "success"
	if err != nil {
		entry.Status = "failed"
	}
	if finishErr := repo.FinishAdminActivity(ctx, entry); finishErr != nil && err == nil {
		return fmt.Errorf("Действие выполнено, но не записано в журнал: %w", finishErr)
	}
	return err
}

func (h *Handler) FindInlinePromo(ctx context.Context, actorID int64, code string) (string, error) {
	if !h.HasAdminPermission(ctx, actorID, "promocodes") {
		return "", errors.New("Нет права выдавать промокоды")
	}
	if h.promoCodeRepository == nil {
		return "", errors.New("Промокоды временно недоступны")
	}
	code = database.NormalizePromoCode(code)
	if !database.IsValidPromoCode(code) {
		return "", errors.New("Введите код промокода")
	}
	promo, err := h.promoCodeRepository.FindByCode(ctx, code)
	if err != nil {
		return "", err
	}
	if promo == nil || !promo.IsActive || promo.DeletedAt != nil || promo.OwnerCustomerID != nil || promo.ExpiresAt != nil && !promo.ExpiresAt.After(time.Now().UTC()) || promo.MaxRedemptions != nil && promo.RedemptionCount >= *promo.MaxRedemptions {
		return "", errors.New("Промокод недоступен")
	}
	return strings.TrimSpace(promo.Code), nil
}

func (h *Handler) SelectedInlineSubscriptionID(ctx context.Context, customerID int64) (int64, error) {
	customer, err := h.customerRepository.FindById(ctx, customerID)
	if err != nil || customer == nil || h.subscriptionRepository == nil {
		return 0, errors.New("Пользователь не найден")
	}
	subscription, err := h.subscriptionRepository.ActiveForCustomer(ctx, customer)
	if err != nil || subscription == nil {
		return 0, errors.New("Подписка не найдена")
	}
	return subscription.ID, nil
}
