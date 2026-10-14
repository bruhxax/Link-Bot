package runtimeconfig

import (
	"fmt"
	"sort"
	"strings"

	planbook "link-bot/internal/plans"
)

const gibibyte = int64(1024 * 1024 * 1024)

func (s *Service) CheckoutPlans() []planbook.CheckoutPlan {
	settings := s.Snapshot()
	result := make([]planbook.CheckoutPlan, 0, len(settings.Plans))
	for _, item := range settings.Plans {
		if !item.Enabled || (item.Months <= 0 && item.Days <= 0) {
			continue
		}
		plan := planbook.CheckoutPlan{
			ID:                       item.ID,
			Name:                     item.Name,
			Months:                   item.Months,
			Days:                     item.Days,
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
		if item.DeviceTrafficEnabled {
			plan.DeviceTrafficEnforced = item.DeviceTrafficEnforced
			plan.DeviceTrafficRules = append([]planbook.DeviceTrafficRule(nil), item.DeviceTrafficRules...)
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

func validateDeviceTraffic(plan *PlanSettings) error {
	if plan.DeviceTrafficEnforced && (!plan.DeviceTrafficEnabled || plan.DeviceLimit < 1 || plan.DeviceLimit > 1000) {
		return fmt.Errorf("для отдельных лимитов включите трафик по устройствам и задайте лимит 1–1000 устройств")
	}
	if len(plan.DeviceTrafficRules) > 100 {
		return fmt.Errorf("не больше 100 строк трафика по устройствам")
	}
	if plan.DeviceTrafficEnabled && len(plan.DeviceTrafficRules) == 0 {
		return fmt.Errorf("добавьте трафик хотя бы для одного количества устройств")
	}
	seen := map[int]bool{}
	for _, rule := range plan.DeviceTrafficRules {
		if plan.DeviceTrafficEnforced && rule.Devices > plan.DeviceLimit {
			return fmt.Errorf("номер устройства превышает лимит устройств тарифа")
		}
		if rule.Devices < 1 || rule.Devices > 1000 || rule.TrafficGB < 1 || rule.TrafficGB > 1000000 {
			return fmt.Errorf("укажите 1–1000 устройств и 1–1000000 ГБ в строках трафика")
		}
		if seen[rule.Devices] {
			return fmt.Errorf("количество устройств в строках трафика не должно повторяться")
		}
		seen[rule.Devices] = true
	}
	sort.Slice(plan.DeviceTrafficRules, func(i, j int) bool { return plan.DeviceTrafficRules[i].Devices < plan.DeviceTrafficRules[j].Devices })
	return nil
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
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = s.Language()
	}
	for _, item := range s.Snapshot().Plans {
		if item.ID != planID {
			continue
		}
		if item.Name != "" {
			return item.Name
		}
		if strings.HasPrefix(strings.ToLower(locale), "fa") {
			return item.TitleFA
		}
		if strings.HasPrefix(strings.ToLower(locale), "en") {
			return item.TitleEN
		}
		return item.TitleRU
	}
	return ""
}
