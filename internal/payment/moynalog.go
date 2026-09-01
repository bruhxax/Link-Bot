package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/moynalog"
	"link-bot/utils"
)

func (s *PaymentService) moyNalogConfigForPurchase(purchase *database.Purchase) (integrations.MoyNalogConfig, bool) {
	if s == nil || purchase == nil || s.integrationSettings == nil || s.moynalogReceiptRepository == nil {
		return integrations.MoyNalogConfig{}, false
	}
	if purchase.Amount <= 0 || purchase.IsFreePlan || purchase.InvoiceType == database.InvoiceTypeFree || purchase.InvoiceType == database.InvoiceTypeBalance {
		return integrations.MoyNalogConfig{}, false
	}
	raw, ok := s.integrationSettings.Config(integrations.ProviderMoyNalog)
	if !ok {
		return integrations.MoyNalogConfig{}, false
	}
	cfg, err := integrations.ParseMoyNalogConfig(raw)
	if err != nil || !cfg.PaymentMethods[string(purchase.InvoiceType)] {
		return integrations.MoyNalogConfig{}, false
	}
	return cfg, true
}

func (s *PaymentService) queueMoyNalogReceipt(purchase *database.Purchase) {
	if _, ok := s.moyNalogConfigForPurchase(purchase); !ok {
		return
	}
	purchaseCopy := *purchase
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := s.createMoyNalogReceipt(ctx, &purchaseCopy, false); err != nil {
			slog.Error("moynalog: receipt creation failed", "error", err, "purchase_id", utils.MaskHalfInt64(purchaseCopy.ID))
		}
	}()
}

func (s *PaymentService) createMoyNalogReceipt(ctx context.Context, purchase *database.Purchase, retry bool) error {
	cfg, ok := s.moyNalogConfigForPurchase(purchase)
	if !ok {
		return errors.New("автоматические чеки выключены для этого способа оплаты")
	}
	itemName := moyNalogReceiptItem(cfg.ItemName, purchase)
	if retry {
		claimed, err := s.moynalogReceiptRepository.RetryFailed(ctx, purchase.ID)
		if err != nil {
			return fmt.Errorf("retry receipt claim: %w", err)
		}
		if !claimed {
			return errors.New("повторная отправка доступна только для чека с подтверждённой ошибкой")
		}
	} else {
		claimed, err := s.moynalogReceiptRepository.Claim(ctx, purchase.ID, purchase.Amount, itemName)
		if err != nil {
			return fmt.Errorf("claim receipt: %w", err)
		}
		if !claimed {
			return nil
		}
	}

	client, err := moynalog.NewClient(cfg.APIURL, cfg.Username, cfg.Password)
	if err != nil {
		s.persistMoyNalogFailure(purchase.ID, database.MoyNalogReceiptFailed, readableMoyNalogError(err))
		return fmt.Errorf("authenticate: %w", err)
	}
	result, err := client.CreateIncome(ctx, purchase.Amount, itemName)
	if err != nil {
		status := database.MoyNalogReceiptFailed
		if errors.Is(err, moynalog.ErrUncertain) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = database.MoyNalogReceiptUncertain
		}
		s.persistMoyNalogFailure(purchase.ID, status, readableMoyNalogError(err))
		return fmt.Errorf("create income: %w", err)
	}
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer persistCancel()
	if err := s.moynalogReceiptRepository.MarkSucceeded(persistCtx, purchase.ID, result.ReceiptUUID()); err != nil {
		return fmt.Errorf("save receipt result: %w", err)
	}
	slog.Info("moynalog: receipt created", "purchase_id", utils.MaskHalfInt64(purchase.ID), "amount", purchase.Amount)
	return nil
}

func (s *PaymentService) persistMoyNalogFailure(purchaseID int64, status, message string) {
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.moynalogReceiptRepository.MarkFailed(persistCtx, purchaseID, status, message); err != nil {
		slog.Error("moynalog: save failure status", "error", err, "purchase_id", utils.MaskHalfInt64(purchaseID))
	}
}

func moyNalogReceiptItem(base string, purchase *database.Purchase) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = integrations.DefaultMoyNalogItemName
	}
	if purchase == nil {
		return base
	}
	result := base
	switch purchase.PurchaseKind {
	case database.PurchaseKindExtraDevices:
		result = fmt.Sprintf("%s — дополнительные устройства (%d)", base, purchase.ExtraDevices)
	case database.PurchaseKindGift:
		result = fmt.Sprintf("%s — подарок на %d мес.", base, purchase.Month)
	default:
		result = fmt.Sprintf("%s на %d мес.", base, purchase.Month)
	}
	runes := []rune(result)
	if len(runes) > 120 {
		result = string(runes[:120])
	}
	return result
}

func readableMoyNalogError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if errors.Is(err, moynalog.ErrAuth) {
		return "Не удалось войти: проверьте ИНН и пароль."
	}
	if errors.Is(err, moynalog.ErrUncertain) {
		return "Ответ ФНС не получен. Проверьте кабинет вручную — повторная отправка заблокирована, чтобы не создать дубликат."
	}
	if errors.Is(err, moynalog.ErrClient) {
		return "ФНС отклонила чек: " + message
	}
	return message
}

func (s *PaymentService) TestMoyNalogConnection(ctx context.Context) error {
	if s == nil || s.integrationSettings == nil {
		return errors.New("интеграции недоступны")
	}
	raw, ok := s.integrationSettings.StoredConfig(integrations.ProviderMoyNalog)
	if !ok {
		return errors.New("сначала сохраните ИНН и пароль")
	}
	cfg, err := integrations.ParseMoyNalogConfig(raw)
	if err != nil {
		return err
	}
	_, err = moynalog.NewClient(cfg.APIURL, cfg.Username, cfg.Password)
	return err
}

func (s *PaymentService) MoyNalogReceipts(ctx context.Context, limit int) ([]database.MoyNalogReceipt, error) {
	if s == nil || s.moynalogReceiptRepository == nil {
		return []database.MoyNalogReceipt{}, nil
	}
	return s.moynalogReceiptRepository.List(ctx, limit)
}

func (s *PaymentService) RetryMoyNalogReceipt(ctx context.Context, purchaseID int64) error {
	if s == nil || s.purchaseRepository == nil || s.moynalogReceiptRepository == nil {
		return errors.New("журнал чеков недоступен")
	}
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return err
	}
	if purchase == nil || purchase.Status != database.PurchaseStatusPaid {
		return errors.New("оплаченная покупка не найдена")
	}
	return s.createMoyNalogReceipt(ctx, purchase, true)
}
