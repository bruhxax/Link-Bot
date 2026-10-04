package miniapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var yandexCounterIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)

type adminYandexPayload struct {
	State       string                  `json:"state"`
	CounterID   string                  `json:"counterId,omitempty"`
	Message     string                  `json:"message,omitempty"`
	ActiveUsers int64                   `json:"activeUsers"`
	NewUsers    int64                   `json:"newUsers"`
	Sessions    int64                   `json:"sessions"`
	PageViews   int64                   `json:"pageViews"`
	Sampled     bool                    `json:"sampled,omitempty"`
	Daily       []adminAnalyticsDaily   `json:"daily"`
	Channels    []adminAnalyticsChannel `json:"channels"`
}

type yandexReportResponse struct {
	Totals  []float64 `json:"totals"`
	Sampled bool      `json:"sampled"`
	Data    []struct {
		Dimensions []struct {
			Name string `json:"name"`
		} `json:"dimensions"`
		Metrics []float64 `json:"metrics"`
	} `json:"data"`
}

type yandexCacheEntry struct {
	value   adminYandexPayload
	expires time.Time
}

var yandexReportCache = struct {
	sync.Mutex
	items map[string]yandexCacheEntry
}{items: make(map[string]yandexCacheEntry)}

func yandexCounterID() string {
	id := strings.TrimSpace(os.Getenv("YANDEX_METRIKA_COUNTER_ID"))
	if yandexCounterIDPattern.MatchString(id) {
		return id
	}
	return ""
}

func (h *Handler) loadAdminYandex(ctx context.Context, from, to time.Time) adminYandexPayload {
	counter := strings.TrimSpace(os.Getenv("YANDEX_METRIKA_COUNTER_ID"))
	if counter == "" {
		return adminYandexPayload{State: "unconfigured", Message: "Добавьте ID счётчика Яндекс Метрики, чтобы собирать посещения и видеть отчёты."}
	}
	if !yandexCounterIDPattern.MatchString(counter) {
		return adminYandexPayload{State: "error", Message: "ID счётчика Яндекс Метрики должен быть положительным числом."}
	}
	token := strings.TrimSpace(os.Getenv("YANDEX_METRIKA_OAUTH_TOKEN"))
	if token == "" {
		return adminYandexPayload{State: "credentials_required", CounterID: counter, Message: "Добавьте OAuth-токен с доступом metrika:read для чтения отчётов."}
	}
	// Token rotation must invalidate cached access without storing the secret in a key.
	key := fmt.Sprintf("%s:%x:%s:%s", counter, sha256.Sum256([]byte(token)), from.Format(adminFinanceDateLayout), to.Format(adminFinanceDateLayout))
	yandexReportCache.Lock()
	entry, cached := yandexReportCache.items[key]
	yandexReportCache.Unlock()
	if cached && time.Now().Before(entry.expires) {
		return entry.value
	}
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	result, err := fetchYandexReport(ctx, http.DefaultClient, counter, token, from, to)
	if err != nil {
		slog.Warn("admin analytics: Yandex report unavailable", "error", err)
		return adminYandexPayload{State: "error", CounterID: counter, Message: "Не удалось получить отчёт. Проверьте OAuth-токен, доступ metrika:read и доступ пользователя к счётчику."}
	}
	yandexReportCache.Lock()
	if len(yandexReportCache.items) >= 64 {
		clear(yandexReportCache.items)
	}
	yandexReportCache.items[key] = yandexCacheEntry{value: result, expires: time.Now().Add(5 * time.Minute)}
	yandexReportCache.Unlock()
	return result
}

func fetchYandexReport(ctx context.Context, client *http.Client, counter, token string, from, to time.Time) (adminYandexPayload, error) {
	start := from.In(adminFinanceLocation).Format(adminFinanceDateLayout)
	end := to.In(adminFinanceLocation).AddDate(0, 0, -1).Format(adminFinanceDateLayout)
	type specification struct {
		dimensions, metrics, sort string
		limit                     int
	}
	specs := []specification{
		{"", "ym:s:users,ym:s:newUsers,ym:s:visits,ym:s:pageviews", "", 1},
		{"ym:s:date", "ym:s:users,ym:s:visits", "ym:s:date", 370},
		{"ym:s:lastTrafficSource", "ym:s:visits", "-ym:s:visits", 10},
	}
	var reports [3]yandexReportResponse
	var errs [3]error
	var wg sync.WaitGroup
	for i, spec := range specs {
		wg.Add(1)
		go func(i int, spec specification) {
			defer wg.Done()
			reports[i], errs[i] = runYandexReport(ctx, client, counter, token, start, end, spec.dimensions, spec.metrics, spec.sort, spec.limit)
		}(i, spec)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return adminYandexPayload{}, err
		}
	}
	if len(reports[0].Totals) < 4 {
		return adminYandexPayload{}, fmt.Errorf("Yandex report is missing summary metrics")
	}
	result := adminYandexPayload{State: "ready", CounterID: counter, ActiveUsers: yandexMetric(reports[0].Totals, 0), NewUsers: yandexMetric(reports[0].Totals, 1), Sessions: yandexMetric(reports[0].Totals, 2), PageViews: yandexMetric(reports[0].Totals, 3), Daily: []adminAnalyticsDaily{}, Channels: []adminAnalyticsChannel{}}
	for _, report := range reports {
		result.Sampled = result.Sampled || report.Sampled
	}
	for _, row := range reports[1].Data {
		if len(row.Dimensions) == 0 {
			continue
		}
		date, err := time.Parse(adminFinanceDateLayout, row.Dimensions[0].Name)
		if err != nil {
			continue
		}
		result.Daily = append(result.Daily, adminAnalyticsDaily{Date: date.Format(adminFinanceDateLayout), Users: yandexMetric(row.Metrics, 0), Sessions: yandexMetric(row.Metrics, 1)})
	}
	for _, row := range reports[2].Data {
		if len(row.Dimensions) == 0 {
			continue
		}
		name := strings.TrimSpace(row.Dimensions[0].Name)
		if name == "" {
			name = "Другое"
		}
		result.Channels = append(result.Channels, adminAnalyticsChannel{Name: name, Sessions: yandexMetric(row.Metrics, 0)})
	}
	return result, nil
}

func yandexMetric(values []float64, index int) int64 {
	if index < 0 || index >= len(values) || math.IsNaN(values[index]) || math.IsInf(values[index], 0) || values[index] < 0 || values[index] >= math.MaxInt64 {
		return 0
	}
	return int64(math.Round(values[index]))
}

func runYandexReport(ctx context.Context, client *http.Client, counter, token, from, to, dimensions, metrics, sort string, limit int) (yandexReportResponse, error) {
	query := url.Values{"ids": {counter}, "date1": {from}, "date2": {to}, "metrics": {metrics}, "limit": {strconv.Itoa(limit)}, "lang": {"ru"}, "timezone": {"+03:00"}, "accuracy": {"full"}}
	if dimensions != "" {
		query.Set("dimensions", dimensions)
	}
	if sort != "" {
		query.Set("sort", sort)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api-metrika.yandex.net/stat/v1/data?"+query.Encode(), nil)
	if err != nil {
		return yandexReportResponse{}, err
	}
	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return yandexReportResponse{}, fmt.Errorf("Yandex report request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return yandexReportResponse{}, fmt.Errorf("Yandex Reporting API returned %d", response.StatusCode)
	}
	var report yandexReportResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&report); err != nil {
		return yandexReportResponse{}, fmt.Errorf("invalid Yandex report response")
	}
	return report, nil
}
