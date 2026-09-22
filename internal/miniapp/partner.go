package miniapp

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

type partnerApplicationRequest struct {
	ResourceURL          string `json:"resourceUrl"`
	RequestedPercent     int    `json:"requestedPercent"`
	ExpectedMonthlyUsers int    `json:"expectedMonthlyUsers"`
}

type adminPartnerReviewRequest struct {
	ApplicationID int64 `json:"applicationId"`
	Approve       bool  `json:"approve"`
	Percent       int   `json:"percent"`
}

type adminPartnerCreateRequest struct {
	TelegramID int64 `json:"telegramId"`
	Percent    int   `json:"percent"`
}

type adminPartnerUpdateRequest struct {
	PartnerID int64  `json:"partnerId"`
	Percent   int    `json:"percent"`
	Active    bool   `json:"active"`
	Action    string `json:"action"`
}

type partnerApplicationPayload struct {
	ID                   int64  `json:"id"`
	ResourceURL          string `json:"resourceUrl"`
	RequestedPercent     int    `json:"requestedPercent"`
	ExpectedMonthlyUsers int    `json:"expectedMonthlyUsers"`
	Status               string `json:"status"`
	CreatedAt            string `json:"createdAt"`
}

type partnerStatsPayload struct {
	Visitors      int     `json:"visitors"`
	TrialUsers    int     `json:"trialUsers"`
	PayingUsers   int     `json:"payingUsers"`
	PurchaseCount int     `json:"purchaseCount"`
	Revenue       float64 `json:"revenue"`
	Commission    float64 `json:"commission"`
	Currency      string  `json:"currency"`
}

type partnerPayload struct {
	ID                int64               `json:"id"`
	CustomerID        int64               `json:"customerId"`
	TelegramID        int64               `json:"telegramId"`
	Username          string              `json:"username"`
	Code              string              `json:"code"`
	CommissionPercent int                 `json:"commissionPercent"`
	IsActive          bool                `json:"isActive"`
	CreatedAt         string              `json:"createdAt"`
	InviteURL         string              `json:"inviteUrl"`
	ShareURL          string              `json:"shareUrl"`
	Stats             partnerStatsPayload `json:"stats"`
}

type partnerMePayload struct {
	Application *partnerApplicationPayload `json:"application,omitempty"`
	Partner     *partnerPayload            `json:"partner,omitempty"`
}

type adminPartnersPayload struct {
	Applications []adminPartnerApplicationPayload `json:"applications"`
	Partners     []partnerPayload                 `json:"partners"`
}

type adminPartnerApplicationPayload struct {
	partnerApplicationPayload
	CustomerID int64  `json:"customerId"`
	TelegramID int64  `json:"telegramId"`
	Username   string `json:"username"`
}

func partnerInviteURL(code string) string {
	botURL := strings.TrimRight(strings.TrimSpace(config.BotURL()), "/")
	code = strings.ToUpper(strings.TrimSpace(code))
	if botURL == "" || code == "" {
		return ""
	}
	return botURL + "?startapp=partner_" + url.QueryEscape(code)
}

func partnerShareURL(code string) string {
	target := partnerInviteURL(code)
	if target == "" {
		return ""
	}
	return "https://t.me/share/url?url=" + url.QueryEscape(target) + "&text=" + url.QueryEscape("Попробуйте VPN по моей партнёрской ссылке")
}

func mapPartnerApplication(item *database.PartnerApplication) *partnerApplicationPayload {
	if item == nil {
		return nil
	}
	return &partnerApplicationPayload{ID: item.ID, ResourceURL: item.ResourceURL, RequestedPercent: item.RequestedPercent, ExpectedMonthlyUsers: item.ExpectedMonthlyUsers, Status: item.Status, CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339)}
}

func (h *Handler) partnerPayload(r *http.Request, partner *database.Partner) (*partnerPayload, error) {
	stats, err := h.partnerRepository.Stats(r.Context(), partner.ID)
	if err != nil {
		return nil, err
	}
	return &partnerPayload{ID: partner.ID, CustomerID: partner.CustomerID, TelegramID: partner.TelegramID, Username: partner.Username, Code: partner.Code, CommissionPercent: partner.CommissionPercent, IsActive: partner.IsActive, CreatedAt: partner.CreatedAt.UTC().Format(time.RFC3339), InviteURL: partnerInviteURL(partner.Code), ShareURL: partnerShareURL(partner.Code), Stats: partnerStatsPayload{Visitors: stats.Visitors, TrialUsers: stats.TrialUsers, PayingUsers: stats.PayingUsers, PurchaseCount: stats.PurchaseCount, Revenue: stats.Revenue, Commission: stats.Commission, Currency: stats.CommissionCurrency}}, nil
}

func (h *Handler) handlePartnerMe(w http.ResponseWriter, r *http.Request, _ *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if h.partnerRepository == nil {
		h.writeError(w, http.StatusServiceUnavailable, "partners_unavailable", "Партнёрская программа временно недоступна")
		return
	}
	application, err := h.partnerRepository.ApplicationForCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_load_failed", "Не удалось загрузить партнёрскую программу")
		return
	}
	partner, err := h.partnerRepository.PartnerForCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_load_failed", "Не удалось загрузить партнёрскую программу")
		return
	}
	payload := partnerMePayload{Application: mapPartnerApplication(application)}
	if partner != nil {
		payload.Partner, err = h.partnerPayload(r, partner)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "partner_load_failed", "Не удалось загрузить статистику")
			return
		}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": payload})
}

func (h *Handler) handlePartnerApply(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	if h.partnerRepository == nil || customer.TelegramIDIsSynthetic {
		h.writeError(w, http.StatusServiceUnavailable, "partners_unavailable", "Партнёрская программа доступна после входа через Telegram")
		return
	}
	var req partnerApplicationRequest
	if err := h.decodeJSONRequest(w, r, 4096, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Проверьте данные заявки")
		return
	}
	resource := strings.TrimSpace(req.ResourceURL)
	parsed, err := url.ParseRequestURI(resource)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || len([]rune(resource)) > 1000 {
		h.writeError(w, http.StatusBadRequest, "partner_resource_invalid", "Укажите корректную ссылку на ресурс")
		return
	}
	if req.RequestedPercent < 0 || req.RequestedPercent > 100 || req.ExpectedMonthlyUsers < 0 || req.ExpectedMonthlyUsers > 10000000 {
		h.writeError(w, http.StatusBadRequest, "partner_application_invalid", "Проверьте процент и прогноз аудитории")
		return
	}
	partner, err := h.partnerRepository.PartnerForCustomer(r.Context(), customer.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_apply_failed", "Не удалось сохранить заявку")
		return
	}
	if partner != nil {
		h.writeError(w, http.StatusConflict, "partner_already_active", "Вы уже участвуете в партнёрской программе")
		return
	}
	application, err := h.partnerRepository.CreateApplication(r.Context(), customer.ID, resource, req.RequestedPercent, req.ExpectedMonthlyUsers)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_apply_failed", "Не удалось сохранить заявку")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Заявка отправлена", "data": partnerMePayload{Application: mapPartnerApplication(application)}})
}

func (h *Handler) adminPartnersAllowed(w http.ResponseWriter, sess *session) bool {
	if h.isAdmin(sess.User.ID) {
		return true
	}
	h.writeError(w, http.StatusForbidden, "forbidden", "Access denied")
	return false
}

func (h *Handler) handleAdminPartnersState(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.adminPartnersAllowed(w, sess) {
		return
	}
	applications, err := h.partnerRepository.ListApplications(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_load_failed", "Не удалось загрузить заявки")
		return
	}
	partners, err := h.partnerRepository.ListPartners(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_load_failed", "Не удалось загрузить партнёров")
		return
	}
	payload := adminPartnersPayload{Applications: make([]adminPartnerApplicationPayload, 0, len(applications)), Partners: make([]partnerPayload, 0, len(partners))}
	for _, application := range applications {
		item := application
		payload.Applications = append(payload.Applications, adminPartnerApplicationPayload{partnerApplicationPayload: *mapPartnerApplication(&item), CustomerID: item.CustomerID, TelegramID: item.TelegramID, Username: item.Username})
	}
	for _, partner := range partners {
		item := partner
		mapped, err := h.partnerPayload(r, &item)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "partner_load_failed", "Не удалось загрузить статистику")
			return
		}
		payload.Partners = append(payload.Partners, *mapped)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": payload})
}

func (h *Handler) handleAdminPartnerReview(w http.ResponseWriter, r *http.Request, sess *session, customer *database.Customer) {
	if !h.adminPartnersAllowed(w, sess) {
		return
	}
	var req adminPartnerReviewRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.ApplicationID <= 0 || (req.Approve && (req.Percent < 0 || req.Percent > 100)) {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Проверьте решение и процент")
		return
	}
	if _, err := h.partnerRepository.ReviewApplication(r.Context(), req.ApplicationID, customer.ID, req.Approve, req.Percent); err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_review_failed", "Не удалось обработать заявку")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) handleAdminPartnerCreate(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.adminPartnersAllowed(w, sess) {
		return
	}
	var req adminPartnerCreateRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.TelegramID <= 0 || req.Percent < 0 || req.Percent > 100 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Укажите Telegram ID и процент от 0 до 100")
		return
	}
	target, err := h.customerRepository.FindByTelegramId(r.Context(), req.TelegramID)
	if err != nil || target == nil {
		h.writeError(w, http.StatusNotFound, "partner_customer_not_found", "Пользователь ещё не зарегистрирован")
		return
	}
	if _, err := h.partnerRepository.CreateOrActivate(r.Context(), target.ID, 0, req.Percent); err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_create_failed", "Не удалось добавить партнёра")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) handleAdminPartnerUpdate(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !h.adminPartnersAllowed(w, sess) {
		return
	}
	var req adminPartnerUpdateRequest
	if err := h.decodeJSONRequest(w, r, 2048, &req); err != nil || req.PartnerID <= 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Выберите партнёра")
		return
	}
	var err error
	if req.Action == "percent" {
		if req.Percent < 0 || req.Percent > 100 {
			h.writeError(w, http.StatusBadRequest, "invalid_percent", "Введите процент от 0 до 100")
			return
		}
		err = h.partnerRepository.SetPercent(r.Context(), req.PartnerID, req.Percent)
	} else {
		err = h.partnerRepository.SetActive(r.Context(), req.PartnerID, req.Active)
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "partner_update_failed", "Не удалось обновить партнёра")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
