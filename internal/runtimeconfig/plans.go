package runtimeconfig

import (
	"strings"

	planbook "link-bot/internal/plans"
)

const gibibyte = int64(1024 * 1024 * 1024)

func (s *Service) CheckoutPlans() []planbook.CheckoutPlan {
	settings := s.Snapshot()
	result := make([]planbook.CheckoutPlan, 0, len(settings.Plans))
	for _, item := range settings.Plans {
		if !item.Enabled || item.Months <= 0 {
			continue
		}
		plan := planbook.CheckoutPlan{
			ID:                       item.ID,
			Months:                   item.Months,
			PriceRub:                 item.PriceRub,
			PriceStars:               planbook.StarsForRub(item.PriceRub),
			FreeOneTime:              item.FreeOneTime,
			DeviceLimitCount:         item.DeviceLimit,
			Wide:                     item.Wide,
			Variant:                  planbook.VariantRegular,
			InternalSquadUUIDs:       append([]string(nil), item.InternalSquadUUIDs...),
			InternalSquadsConfigured: item.InternalSquadsConfigured,
			ExternalSquadUUID:        item.ExternalSquadUUID,
		}
		if item.UnlimitedTraffic || item.TrafficGB == 0 {
			plan.TrafficLimitBytes = 0
			plan.Variant = planbook.VariantUnlimited
		} else {
			plan.TrafficLimitBytes = int64(item.TrafficGB) * gibibyte
		}
		result = append(result, plan)
	}
	return result
}

func (s *Service) CheckoutPlan(planID string, months int) (planbook.CheckoutPlan, bool) {
	planID = strings.TrimSpace(planID)
	if planID == "" && months > 0 {
		planID = planbook.RegularIDForMonths(months)
	}
	for _, plan := range s.CheckoutPlans() {
		if plan.ID == planID {
			return plan, true
		}
	}
	return planbook.CheckoutPlan{}, false
}

func (s *Service) PlanTitle(planID, locale string) string {
	for _, item := range s.Snapshot().Plans {
		if item.ID != planID {
			continue
		}
		if strings.HasPrefix(strings.ToLower(locale), "en") {
			return item.TitleEN
		}
		return item.TitleRU
	}
	return ""
}
