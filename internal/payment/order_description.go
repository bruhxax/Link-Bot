package payment

import (
	"fmt"
	"strings"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

const orderDescriptionLimit = 120 // YooKassa payment and receipt descriptions allow 128 characters.
const descriptionGB = int64(1024 * 1024 * 1024)

// BuildOrderDescription describes exactly the entitlement being purchased.
// Keep it short enough to fit the strictest customer-facing payment field.
func BuildOrderDescription(months int, options CreatePurchaseOptions) string {
	parts := make([]string, 0, 6)
	switch options.PurchaseKind {
	case database.PurchaseKindExtraDevices:
		parts = append(parts, "Дополнительно +"+formatOrderDevices(options.ExtraDevices))
		if options.DeviceExpiresAt != nil && !options.DeviceExpiresAt.IsZero() {
			parts = append(parts, "до "+options.DeviceExpiresAt.Format("02.01.2006"))
		}
		return limitOrderDescription(strings.Join(parts, " · "))
	case database.PurchaseKindExtraTraffic:
		if options.ExtraTrafficUnlimited {
			return "Дополнительный безлимитный трафик · без сброса"
		}
		return limitOrderDescription("Дополнительный трафик: +" + formatOrderTraffic(options.ExtraTrafficBytes) + " · без сброса")
	}

	if months > 0 || options.DurationDays > 0 {
		parts = append(parts, paymentDurationDescription(months, options.DurationDays))
	} else {
		parts = append(parts, "VPN-подписка")
	}
	devices := config.DeviceLimitForMonths(months)
	if options.DeviceLimitCount != nil {
		devices = *options.DeviceLimitCount
	}
	if devices > 0 {
		parts = append(parts, formatOrderDevices(devices))
	} else if options.DeviceLimitCount != nil {
		parts = append(parts, "устройства без лимита")
	}
	traffic := int64(config.TrafficLimitForMonths(months))
	if options.TrafficLimitBytes != nil {
		traffic = *options.TrafficLimitBytes
	}
	if traffic > 0 {
		parts = append(parts, "трафик "+formatOrderTraffic(traffic))
	} else if options.TrafficLimitBytes != nil {
		parts = append(parts, "безлимитный трафик")
	}
	if options.ExtraDevices > 0 {
		parts = append(parts, "+"+formatOrderDevices(options.ExtraDevices))
	}
	if options.ExtraTrafficUnlimited {
		parts = append(parts, "+безлимитный трафик")
	} else if options.ExtraTrafficBytes > 0 {
		parts = append(parts, "+"+formatOrderTraffic(options.ExtraTrafficBytes)+" трафика")
	}
	if options.PromoDiscountPercent > 0 {
		parts = append(parts, fmt.Sprintf("скидка %d%%", options.PromoDiscountPercent))
	}
	description := strings.Join(parts, " · ")
	if options.PurchaseKind == database.PurchaseKindGift {
		description = "Подарок: " + description
	}
	return limitOrderDescription(description)
}

func purchaseOrderDescription(purchase *database.Purchase) string {
	if purchase == nil {
		return "VPN-подписка"
	}
	promoDiscount := 0
	if purchase.PromoCodeDiscountPercent != nil {
		promoDiscount = *purchase.PromoCodeDiscountPercent
	}
	return BuildOrderDescription(purchase.Month, CreatePurchaseOptions{
		DurationDays:          purchase.Days,
		TrafficLimitBytes:     purchase.TrafficLimitBytes,
		DeviceLimitCount:      purchase.DeviceLimitCount,
		PromoDiscountPercent:  promoDiscount,
		PurchaseKind:          purchase.PurchaseKind,
		ExtraDevices:          purchase.ExtraDevices,
		ExtraTrafficBytes:     purchase.ExtraTrafficBytes,
		ExtraTrafficUnlimited: purchase.ExtraTrafficUnlimited,
		DeviceExpiresAt:       purchase.DeviceExpiresAt,
	})
}

func formatOrderTraffic(bytes int64) string {
	if bytes%descriptionGB == 0 {
		return fmt.Sprintf("%d ГБ", bytes/descriptionGB)
	}
	return fmt.Sprintf("%.2f ГБ", float64(bytes)/float64(descriptionGB))
}

func formatOrderDevices(count int) string {
	word := "устройств"
	if count%100 < 11 || count%100 > 14 {
		switch count % 10 {
		case 1:
			word = "устройство"
		case 2, 3, 4:
			word = "устройства"
		}
	}
	return fmt.Sprintf("%d %s", count, word)
}

func limitOrderDescription(description string) string {
	value := []rune(strings.TrimSpace(description))
	if len(value) > orderDescriptionLimit {
		return string(value[:orderDescriptionLimit])
	}
	return string(value)
}
