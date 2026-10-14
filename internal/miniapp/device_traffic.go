package miniapp

import (
	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
)

// Use the selected subscription's purchased plan, never the plan selected for
// the next checkout. Disabled plans can still describe an existing subscription.
func subscriptionDeviceTrafficRules(plans []runtimeconfig.PlanSettings, planID string) []planbook.DeviceTrafficRule {
	for _, plan := range plans {
		if plan.ID == planID && plan.DeviceTrafficEnabled {
			return append([]planbook.DeviceTrafficRule(nil), plan.DeviceTrafficRules...)
		}
	}
	return nil
}
