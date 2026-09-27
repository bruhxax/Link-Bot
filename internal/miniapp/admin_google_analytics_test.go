package miniapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type ga4RoundTripFunc func(*http.Request) (*http.Response, error)

func (f ga4RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRunGA4ReportRequest(t *testing.T) {
	client := &http.Client{Transport: ga4RoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://analyticsdata.googleapis.com/v1beta/properties/12345:runReport" {
			t.Fatalf("unexpected GA4 endpoint: %s", request.URL)
		}
		var body struct {
			DateRanges []struct {
				StartDate string `json:"startDate"`
				EndDate   string `json:"endDate"`
			} `json:"dateRanges"`
			Dimensions []struct {
				Name string `json:"name"`
			} `json:"dimensions"`
			Metrics []struct {
				Name string `json:"name"`
			} `json:"metrics"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.DateRanges) != 1 || body.DateRanges[0].StartDate != "2026-09-01" || body.DateRanges[0].EndDate != "2026-09-28" {
			t.Fatalf("unexpected GA4 dates: %+v", body.DateRanges)
		}
		if len(body.Dimensions) != 1 || body.Dimensions[0].Name != "date" || len(body.Metrics) != 1 || body.Metrics[0].Name != "sessions" {
			t.Fatalf("unexpected GA4 report fields: %+v %+v", body.Dimensions, body.Metrics)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"rows":[{"dimensionValues":[{"value":"20260928"}],"metricValues":[{"value":"7"}]}]}`))}, nil
	})}
	report, err := runGA4Report(context.Background(), client, "12345", "2026-09-01", "2026-09-28", []string{"date"}, []string{"sessions"}, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 1 || len(report.Rows[0].Metrics) != 1 || report.Rows[0].Metrics[0].Value != "7" {
		t.Fatalf("unexpected GA4 response: %+v", report)
	}
}
