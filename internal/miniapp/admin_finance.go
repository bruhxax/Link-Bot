package miniapp

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"link-bot/internal/database"
)

const adminFinanceDateLayout = "2006-01-02"

var adminFinanceLocation = time.FixedZone("MSK", 3*60*60)

type adminFinanceRequest struct {
	Period string `json:"period"`
	From   string `json:"from"`
	To     string `json:"to"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type adminFinanceSummaryPayload struct {
	RevenueRub   float64 `json:"revenueRub"`
	RefundsRub   float64 `json:"refundsRub"`
	RevenueStars float64 `json:"revenueStars"`
	RefundsStars float64 `json:"refundsStars"`
	PaymentCount int     `json:"paymentCount"`
}

type adminFinanceDailyPayload struct {
	Date         string  `json:"date"`
	RevenueRub   float64 `json:"revenueRub"`
	RefundsRub   float64 `json:"refundsRub"`
	RevenueStars float64 `json:"revenueStars"`
	RefundsStars float64 `json:"refundsStars"`
	PaymentCount int     `json:"paymentCount"`
}

type adminFinancePaymentPayload struct {
	ID         int64   `json:"id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	Status     string  `json:"status"`
	Provider   string  `json:"provider"`
	Plan       string  `json:"plan"`
	Username   string  `json:"username"`
	TelegramID int64   `json:"telegramId"`
	OccurredAt string  `json:"occurredAt"`
}

type adminFinancePayload struct {
	Period       string                       `json:"period"`
	From         string                       `json:"from"`
	To           string                       `json:"to"`
	Summary      adminFinanceSummaryPayload   `json:"summary"`
	Daily        []adminFinanceDailyPayload   `json:"daily"`
	Payments     []adminFinancePaymentPayload `json:"payments"`
	PaymentTotal int                          `json:"paymentTotal"`
	Limit        int                          `json:"limit"`
	Offset       int                          `json:"offset"`
}

func resolveAdminFinanceRange(req adminFinanceRequest, now time.Time) (string, time.Time, time.Time, error) {
	now = now.In(adminFinanceLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, adminFinanceLocation)
	period := strings.ToLower(strings.TrimSpace(req.Period))
	if period == "" {
		period = "7d"
	}
	var fromDate, toDate time.Time
	switch period {
	case "7d", "30d", "90d", "365d":
		days := map[string]int{"7d": 7, "30d": 30, "90d": 90, "365d": 365}[period]
		fromDate = today.AddDate(0, 0, -(days - 1))
		toDate = today
	case "month":
		fromDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, adminFinanceLocation)
		toDate = today
	case "custom":
		var err error
		fromDate, err = time.ParseInLocation(adminFinanceDateLayout, strings.TrimSpace(req.From), adminFinanceLocation)
		if err != nil {
			return "", time.Time{}, time.Time{}, fmt.Errorf("invalid from date")
		}
		toDate, err = time.ParseInLocation(adminFinanceDateLayout, strings.TrimSpace(req.To), adminFinanceLocation)
		if err != nil {
			return "", time.Time{}, time.Time{}, fmt.Errorf("invalid to date")
		}
		if toDate.After(today) {
			toDate = today
		}
		if fromDate.After(toDate) || toDate.Sub(fromDate) > 365*24*time.Hour {
			return "", time.Time{}, time.Time{}, fmt.Errorf("invalid custom range")
		}
	default:
		return "", time.Time{}, time.Time{}, fmt.Errorf("invalid finance period")
	}
	return period, fromDate.UTC(), toDate.AddDate(0, 0, 1).UTC(), nil
}

func adminFinanceProvider(item database.AdminFinancePayment) string {
	if item.InvoiceType == database.InvoiceTypeYookasa && item.YookasaPaymentMethodTitle != "" {
		return item.YookasaPaymentMethodTitle
	}
	label := map[database.InvoiceType]string{
		database.InvoiceTypeCrypto:    "Crypto Pay",
		database.InvoiceTypeYookasa:   "YooKassa",
		database.InvoiceTypeTelegram:  "Telegram Stars",
		database.InvoiceTypeTribute:   "Tribute",
		database.InvoiceTypeLava:      "LAVA",
		database.InvoiceTypeWata:      "WATA",
		database.InvoiceTypePlatega:   "Platega",
		database.InvoiceTypeFreeKassa: "FreeKassa",
		database.InvoiceTypeHeleket:   "Heleket",
		database.InvoiceTypePally:     "Pally",
		database.InvoiceTypeP2P:       "P2P",
		database.InvoiceTypeBalance:   "Баланс",
	}[item.InvoiceType]
	if label != "" {
		return label
	}
	return strings.ToUpper(strings.TrimSpace(string(item.InvoiceType)))
}

func adminFinancePlan(item database.AdminFinancePayment) string {
	switch item.PurchaseKind {
	case database.PurchaseKindExtraDevices:
		count := item.ExtraDevices
		if count <= 0 {
			count = 1
		}
		return fmt.Sprintf("Дополнительные устройства · %d", count)
	case database.PurchaseKindGift:
		return fmt.Sprintf("Подарок · %s", adminFinanceMonthLabel(item.Month))
	default:
		return "Подписка на " + adminFinanceMonthLabel(item.Month)
	}
}

func adminFinanceMonthLabel(months int) string {
	if months <= 0 {
		return "1 месяц"
	}
	lastTwo := months % 100
	last := months % 10
	word := "месяцев"
	if lastTwo < 11 || lastTwo > 14 {
		if last == 1 {
			word = "месяц"
		} else if last >= 2 && last <= 4 {
			word = "месяца"
		}
	}
	return fmt.Sprintf("%d %s", months, word)
}

func adminFinanceStatus(item database.AdminFinancePayment) string {
	if item.Status == database.PurchaseStatusCancel && item.WasPaid {
		return "refunded"
	}
	return string(item.Status)
}

func (h *Handler) handleAdminFinance(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.isAdmin(sess.User.ID) {
		h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}
	var req adminFinanceRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	period, from, to, err := resolveAdminFinanceRange(req, time.Now())
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_finance_period", "Выберите период не больше 366 дней")
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 30
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	data, err := h.purchaseRepository.LoadAdminFinance(r.Context(), from, to, req.Limit, req.Offset)
	if err != nil {
		slog.Error("mini app: load admin finance", "error", err)
		h.writeError(w, http.StatusInternalServerError, "admin_finance_failed", "Не удалось загрузить финансы")
		return
	}

	dailyByDate := make(map[string]database.AdminFinanceDaily, len(data.Daily))
	for _, item := range data.Daily {
		dailyByDate[item.Date.Format(adminFinanceDateLayout)] = item
	}
	payload := adminFinancePayload{
		Period: period,
		From:   from.In(adminFinanceLocation).Format(adminFinanceDateLayout),
		To:     to.In(adminFinanceLocation).AddDate(0, 0, -1).Format(adminFinanceDateLayout),
		Summary: adminFinanceSummaryPayload{
			RevenueRub:   data.Summary.RevenueRub,
			RefundsRub:   data.Summary.RefundsRub,
			RevenueStars: data.Summary.RevenueStars,
			RefundsStars: data.Summary.RefundsStars,
			PaymentCount: data.Summary.PaymentCount,
		},
		Daily:        make([]adminFinanceDailyPayload, 0, int(to.Sub(from).Hours()/24)),
		Payments:     make([]adminFinancePaymentPayload, 0, len(data.Payments)),
		PaymentTotal: data.PaymentTotal,
		Limit:        req.Limit,
		Offset:       req.Offset,
	}
	for day := from.In(adminFinanceLocation); day.Before(to.In(adminFinanceLocation)); day = day.AddDate(0, 0, 1) {
		date := day.Format(adminFinanceDateLayout)
		item := dailyByDate[date]
		payload.Daily = append(payload.Daily, adminFinanceDailyPayload{
			Date:         date,
			RevenueRub:   item.RevenueRub,
			RefundsRub:   item.RefundsRub,
			RevenueStars: item.RevenueStars,
			RefundsStars: item.RefundsStars,
			PaymentCount: item.PaymentCount,
		})
	}
	for _, item := range data.Payments {
		payload.Payments = append(payload.Payments, adminFinancePaymentPayload{
			ID:         item.ID,
			Amount:     item.Amount,
			Currency:   item.Currency,
			Status:     adminFinanceStatus(item),
			Provider:   adminFinanceProvider(item),
			Plan:       adminFinancePlan(item),
			Username:   item.TelegramUsername,
			TelegramID: item.TelegramID,
			OccurredAt: item.OccurredAt.UTC().Format(time.RFC3339),
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": payload})
}
