package miniapp

import (
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"strings"
)

// A previous trial activation alone is insufficient: a later purchase wins.
// For imported/manual accounts an explicit configured trial tag is evidence.
func panelIsTrial(user remnawave.PanelUser, observation database.PanelPlanObservation, trial runtimeconfig.TrialSettings) bool {
	if observation.Months > 0 || observation.Days > 0 {
		return false
	}
	if user.Tag != nil && strings.TrimSpace(trial.Tag) != "" && strings.EqualFold(strings.TrimSpace(*user.Tag), strings.TrimSpace(trial.Tag)) {
		return true
	}
	if !observation.IsTrial {
		return false
	}
	traffic := int64(trial.TrafficGB) * 1024 * 1024 * 1024
	if trial.UnlimitedTraffic {
		traffic = 0
	}
	return user.TrafficLimitBytes == traffic && (user.HwidDeviceLimit == nil || *user.HwidDeviceLimit == trial.DeviceLimit)
}

func panelTrialLabel(unlimited bool) string {
	if unlimited {
		return "Пробный период · Безлимит"
	}
	return "Пробный период · Лимитный"
}
