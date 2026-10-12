package miniapp

import (
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"strings"
	"testing"
	"time"
)

func TestTrialTariffUsesActivationOrExplicitPanelTagWithoutRelabelingPaidPlans(t *testing.T) {
	trialTag := "TRIAL"
	devices := 2
	now := time.Now()
	settings := runtimeconfig.Settings{Trial: runtimeconfig.TrialSettings{Days: 3, TrafficGB: 10, DeviceLimit: 2, Tag: trialTag}}
	user := remnawave.PanelUser{ID: 1, Status: "ACTIVE", ExpireAt: now.Add(time.Hour), TrafficLimitBytes: 10 * 1024 * 1024 * 1024, HwidDeviceLimit: &devices}
	observation := database.PanelPlanObservation{IsTrial: true}
	for _, tc := range []struct {
		name    string
		user    remnawave.PanelUser
		binding database.PanelPlanObservation
		want    bool
	}{
		{"linked primary trial", user, observation, true},
		{"additional subscription with similar limits", user, database.PanelPlanObservation{}, false},
		{"paid after trial", user, database.PanelPlanObservation{IsTrial: true, Months: 1}, false},
		{"paid day tariff", user, database.PanelPlanObservation{IsTrial: true, Days: 3}, false},
		{"manually increased traffic", remnawave.PanelUser{TrafficLimitBytes: 50 * 1024 * 1024 * 1024, HwidDeviceLimit: &devices}, observation, false},
		{"imported tagged trial", remnawave.PanelUser{Tag: &trialTag}, database.PanelPlanObservation{}, true},
		{"paid tariff retains old tag", remnawave.PanelUser{Tag: &trialTag}, database.PanelPlanObservation{Months: 3}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := panelIsTrial(tc.user, tc.binding, settings.Trial); got != tc.want {
				t.Fatalf("isTrial=%v want %v", got, tc.want)
			}
		})
	}
	id := int64(1)
	observation.PanelID = &id
	catalog := panelCatalogSnapshot{Users: []remnawave.PanelUser{user}, Observations: []database.PanelPlanObservation{observation}}
	rows, _ := panelUserRows(catalog, "", settings)
	_, plans := panelOverview(catalog.Users, catalog.Observations, settings, 30, now)
	if len(rows) != 1 || !strings.HasPrefix(rows[0].PlanTitle, "Пробный период") || len(plans) != 1 || !plans[0].IsTrial || !strings.HasPrefix(plans[0].Label, "Пробный период") {
		t.Fatalf("list and overview disagree: %+v %+v", rows, plans)
	}
	user.Status = "EXPIRED"
	user.ExpireAt = now.Add(-time.Hour)
	rows, _ = panelUserRows(panelCatalogSnapshot{Users: []remnawave.PanelUser{user}, Observations: catalog.Observations}, "Пробный", settings)
	if len(rows) != 1 {
		t.Fatal("expired trial lost its tariff label/search")
	}
}
