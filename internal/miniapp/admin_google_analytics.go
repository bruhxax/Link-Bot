package miniapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"link-bot/internal/database"
)

var ga4PropertyIDPattern = regexp.MustCompile(`^[0-9]+$`)
var ga4MeasurementIDPattern = regexp.MustCompile(`^G-[A-Z0-9]+$`)

type adminGA4Payload struct {
	State       string                  `json:"state"`
	PropertyID  string                  `json:"propertyId,omitempty"`
	Message     string                  `json:"message,omitempty"`
	ActiveUsers int64                   `json:"activeUsers"`
	NewUsers    int64                   `json:"newUsers"`
	Sessions    int64                   `json:"sessions"`
	PageViews   int64                   `json:"pageViews"`
	Daily       []adminAnalyticsDaily   `json:"daily"`
	Channels    []adminAnalyticsChannel `json:"channels"`
}

type adminAnalyticsDaily struct {
	Date     string `json:"date"`
	Users    int64  `json:"users"`
	Sessions int64  `json:"sessions"`
}

type adminAnalyticsChannel struct {
	Name     string `json:"name"`
	Sessions int64  `json:"sessions"`
}

type ga4ReportResponse struct {
	Rows []struct {
		Dimensions []struct {
			Value string `json:"value"`
		} `json:"dimensionValues"`
		Metrics []struct {
			Value string `json:"value"`
		} `json:"metricValues"`
	} `json:"rows"`
}

type ga4CacheEntry struct {
	value   adminGA4Payload
	expires time.Time
}

var ga4ReportCache = struct {
	sync.Mutex
	items map[string]ga4CacheEntry
}{items: make(map[string]ga4CacheEntry)}

func ga4MeasurementID() string {
	id := strings.ToUpper(strings.TrimSpace(os.Getenv("GA4_MEASUREMENT_ID")))
	if ga4MeasurementIDPattern.MatchString(id) {
		return id
	}
	return ""
}

func (h *Handler) loadAdminGA4(ctx context.Context, from, to time.Time) adminGA4Payload {
	property := strings.TrimSpace(os.Getenv("GA4_PROPERTY_ID"))
	if property == "" {
		return adminGA4Payload{State: "unconfigured", Message: "Создайте ресурс GA4 и укажите его ID, чтобы видеть посещения здесь."}
	}
	if !ga4PropertyIDPattern.MatchString(property) {
		return adminGA4Payload{State: "error", Message: "ID ресурса GA4 должен содержать только цифры."}
	}
	credentialsPath := strings.TrimSpace(os.Getenv("GA4_SERVICE_ACCOUNT_FILE"))
	if credentialsPath == "" {
		return adminGA4Payload{State: "credentials_required", PropertyID: property, Message: "Добавьте ключ сервисного аккаунта для чтения отчётов GA4."}
	}
	key := property + ":" + from.Format("2006-01-02") + ":" + to.Format("2006-01-02")
	ga4ReportCache.Lock()
	entry, cached := ga4ReportCache.items[key]
	ga4ReportCache.Unlock()
	if cached && time.Now().Before(entry.expires) {
		return entry.value
	}

	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	result, err := fetchGA4Report(ctx, property, credentialsPath, from, to)
	if err != nil {
		slog.Warn("admin finance: Google Analytics report unavailable", "error", err)
		return adminGA4Payload{State: "error", PropertyID: property, Message: "Не удалось получить отчёт. Проверьте доступ сервисного аккаунта к GA4 и включение Data API."}
	}
	ga4ReportCache.Lock()
	ga4ReportCache.items[key] = ga4CacheEntry{value: result, expires: time.Now().Add(5 * time.Minute)}
	ga4ReportCache.Unlock()
	return result
}

func (h *Handler) handleAdminAnalytics(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
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
		h.writeError(w, http.StatusBadRequest, "invalid_analytics_period", "Выберите период не больше 366 дней")
		return
	}
	var googleReport adminGA4Payload
	var yandexReport adminYandexPayload
	var reports sync.WaitGroup
	reports.Add(2)
	go func() { defer reports.Done(); googleReport = h.loadAdminGA4(r.Context(), from, to) }()
	go func() { defer reports.Done(); yandexReport = h.loadAdminYandex(r.Context(), from, to) }()
	reports.Wait()
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{
		"period": period,
		"from":   from.In(adminFinanceLocation).Format(adminFinanceDateLayout),
		"to":     to.In(adminFinanceLocation).AddDate(0, 0, -1).Format(adminFinanceDateLayout),
		"google": googleReport,
		"yandex": yandexReport,
	}})
}

func fetchGA4Report(ctx context.Context, property, credentialsPath string, from, to time.Time) (adminGA4Payload, error) {
	secret, err := os.ReadFile(credentialsPath)
	if err != nil {
		return adminGA4Payload{}, fmt.Errorf("read GA4 service account: %w", err)
	}
	credentials, err := google.CredentialsFromJSON(ctx, secret, "https://www.googleapis.com/auth/analytics.readonly")
	if err != nil {
		return adminGA4Payload{}, fmt.Errorf("parse GA4 service account: %w", err)
	}
	client := oauth2.NewClient(ctx, credentials.TokenSource)
	startDate := from.In(adminFinanceLocation).Format(adminFinanceDateLayout)
	endDate := to.In(adminFinanceLocation).AddDate(0, 0, -1).Format(adminFinanceDateLayout)
	request := func(dimensions, metrics []string, limit int) (ga4ReportResponse, error) {
		return runGA4Report(ctx, client, property, startDate, endDate, dimensions, metrics, limit)
	}
	summary, err := request(nil, []string{"activeUsers", "newUsers", "sessions", "screenPageViews"}, 1)
	if err != nil {
		return adminGA4Payload{}, err
	}
	daily, err := request([]string{"date"}, []string{"activeUsers", "sessions"}, 370)
	if err != nil {
		return adminGA4Payload{}, err
	}
	channels, err := request([]string{"sessionDefaultChannelGroup"}, []string{"sessions"}, 10)
	if err != nil {
		return adminGA4Payload{}, err
	}
	result := adminGA4Payload{State: "ready", PropertyID: property, Daily: make([]adminAnalyticsDaily, 0, len(daily.Rows)), Channels: make([]adminAnalyticsChannel, 0, len(channels.Rows))}
	if len(summary.Rows) > 0 {
		metrics := summary.Rows[0].Metrics
		result.ActiveUsers = ga4Metric(metrics, 0)
		result.NewUsers = ga4Metric(metrics, 1)
		result.Sessions = ga4Metric(metrics, 2)
		result.PageViews = ga4Metric(metrics, 3)
	}
	for _, row := range daily.Rows {
		if len(row.Dimensions) == 0 || len(row.Dimensions[0].Value) != 8 {
			continue
		}
		value := row.Dimensions[0].Value
		result.Daily = append(result.Daily, adminAnalyticsDaily{Date: value[:4] + "-" + value[4:6] + "-" + value[6:], Users: ga4Metric(row.Metrics, 0), Sessions: ga4Metric(row.Metrics, 1)})
	}
	for _, row := range channels.Rows {
		if len(row.Dimensions) == 0 {
			continue
		}
		result.Channels = append(result.Channels, adminAnalyticsChannel{Name: row.Dimensions[0].Value, Sessions: ga4Metric(row.Metrics, 0)})
	}
	return result, nil
}

func ga4Metric(values []struct {
	Value string `json:"value"`
}, index int) int64 {
	if index < 0 || index >= len(values) {
		return 0
	}
	value, _ := strconv.ParseInt(values[index].Value, 10, 64)
	return value
}

func runGA4Report(ctx context.Context, client *http.Client, property, from, to string, dimensions, metrics []string, limit int) (ga4ReportResponse, error) {
	type named struct {
		Name string `json:"name"`
	}
	type dateRange struct {
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
	}
	requestBody := struct {
		DateRanges []dateRange `json:"dateRanges"`
		Dimensions []named     `json:"dimensions,omitempty"`
		Metrics    []named     `json:"metrics"`
		Limit      string      `json:"limit"`
	}{Limit: strconv.Itoa(limit)}
	requestBody.DateRanges = append(requestBody.DateRanges, dateRange{from, to})
	for _, dimension := range dimensions {
		requestBody.Dimensions = append(requestBody.Dimensions, named{Name: dimension})
	}
	for _, metric := range metrics {
		requestBody.Metrics = append(requestBody.Metrics, named{Name: metric})
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return ga4ReportResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://analyticsdata.googleapis.com/v1beta/properties/"+property+":runReport", bytes.NewReader(body))
	if err != nil {
		return ga4ReportResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return ga4ReportResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ga4ReportResponse{}, fmt.Errorf("GA4 Data API returned %d", response.StatusCode)
	}
	var report ga4ReportResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&report); err != nil {
		return ga4ReportResponse{}, err
	}
	return report, nil
}
