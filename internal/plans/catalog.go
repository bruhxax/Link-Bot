package plans

import (
	"math"
	"strings"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

const RublesPerStar = 1.47

const (
	VariantRegular   = "regular"
	VariantUnlimited = "unlimited"

	ID1Month           = "1m"
	ID1MonthUnlimited  = "1m_unlimited"
	ID3Months          = "3m"
	ID3MonthsUnlimited = "3m_unlimited"
	ID6Months          = "6m"
	ID6MonthsUnlimited = "6m_unlimited"
	ID12Months         = "12m"

	UnlimitedPrice1Month = 170
	UnlimitedPrice3Month = 320
	UnlimitedPrice6Month = 430
)

type CheckoutPlan struct {
	ID                 string
	Months             int
	PriceRub           int
	PriceStars         int
	TrafficLimitBytes  int64
	DeviceLimitCount   int
	Variant            string
	Wide               bool
	InternalSquadUUIDs []string
	ExternalSquadUUID  string
}

func StarsForRub(priceRub int) int {
	if priceRub <= 0 {
		return 0
	}
	return int(math.Round(float64(priceRub) / RublesPerStar))
}

func All() []CheckoutPlan {
	return []CheckoutPlan{
		regular(ID1Month, 1, config.Price1(), StarsForRub(config.Price1())),
		unlimited(ID1MonthUnlimited, 1, UnlimitedPrice1Month, config.DeviceLimitForMonths(1)),
		regular(ID3Months, 3, config.Price3(), StarsForRub(config.Price3())),
		unlimited(ID3MonthsUnlimited, 3, UnlimitedPrice3Month, config.DeviceLimitForMonths(3)),
		regular(ID6Months, 6, config.Price6(), StarsForRub(config.Price6())),
		unlimited(ID6MonthsUnlimited, 6, UnlimitedPrice6Month, config.DeviceLimitForMonths(6)),
		regular(ID12Months, 12, config.Price12(), StarsForRub(config.Price12())).withWide(),
	}
}

func ForIDOrMonths(planID string, months int) (CheckoutPlan, bool) {
	planID = strings.TrimSpace(planID)
	if planID == "" && months > 0 {
		planID = RegularIDForMonths(months)
	}

	for _, plan := range All() {
		if plan.ID != planID {
			continue
		}
		return plan, plan.PriceRub > 0 || plan.PriceStars > 0
	}

	return CheckoutPlan{}, false
}

func RegularIDForMonths(months int) string {
	switch months {
	case 1:
		return ID1Month
	case 3:
		return ID3Months
	case 6:
		return ID6Months
	case 12:
		return ID12Months
	default:
		return ""
	}
}

func AmountForInvoice(plan CheckoutPlan, invoiceType database.InvoiceType) (int, bool) {
	switch invoiceType {
	case database.InvoiceTypeTelegram:
		return plan.PriceStars, plan.PriceStars > 0
	default:
		return plan.PriceRub, plan.PriceRub > 0
	}
}

func regular(id string, months int, priceRub int, priceStars int) CheckoutPlan {
	return CheckoutPlan{
		ID:                id,
		Months:            months,
		PriceRub:          priceRub,
		PriceStars:        priceStars,
		TrafficLimitBytes: int64(config.TrafficLimitForMonths(months)),
		DeviceLimitCount:  config.DeviceLimitForMonths(months),
		Variant:           VariantRegular,
	}
}

func unlimited(id string, months int, priceRub int, deviceLimit int) CheckoutPlan {
	return CheckoutPlan{
		ID:                id,
		Months:            months,
		PriceRub:          priceRub,
		PriceStars:        0,
		TrafficLimitBytes: 0,
		DeviceLimitCount:  deviceLimit,
		Variant:           VariantUnlimited,
	}
}

func (p CheckoutPlan) withWide() CheckoutPlan {
	p.Wide = true
	return p
}
