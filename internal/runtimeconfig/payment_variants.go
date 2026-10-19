package runtimeconfig

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"link-bot/internal/paymentchannel"
)

// PaymentMethodVariant is either a channel override for a built-in row or an
// independently named and ordered copy. Copies share integration credentials.
type PaymentMethodVariant struct {
	Method   string   `json:"method"`
	Channels []string `json:"channels"`
}

var paymentCopyID = regexp.MustCompile(`^[a-z]+__[a-f0-9]{8}$`)

func (s Settings) PaymentMethodBase(id string) string {
	if variant, ok := s.PaymentMethodVariants[id]; ok {
		return variant.Method
	}
	if contains(defaultPaymentMethodOrder, id) {
		return id
	}
	return ""
}

func normalizePaymentVariants(s *Settings) error {
	if len(s.PaymentMethodVariants) > 96 {
		return fmt.Errorf("слишком много вариантов оплаты: максимум 96")
	}
	clean := make(map[string]PaymentMethodVariant, len(s.PaymentMethodVariants))
	for id, variant := range s.PaymentMethodVariants {
		variant.Method = strings.TrimSpace(variant.Method)
		if !contains(defaultPaymentMethodOrder, variant.Method) {
			return fmt.Errorf("неизвестная платёжка: %s", variant.Method)
		}
		if id != variant.Method && (!paymentCopyID.MatchString(id) || paymentchannel.Base(id) != variant.Method) {
			return fmt.Errorf("некорректный вариант оплаты: %s", id)
		}
		if id != variant.Method && (variant.Method == "balance" || variant.Method == "stars" || variant.Method == "p2p") {
			return fmt.Errorf("этот способ оплаты не требует копий")
		}
		if len(variant.Channels) > 256 {
			return fmt.Errorf("слишком много методов оплаты")
		}
		channels := []string{}
		seen := map[string]bool{}
		for _, channel := range variant.Channels {
			channel = strings.TrimSpace(channel)
			if !paymentchannel.Valid(variant.Method, channel) {
				return fmt.Errorf("%s: неподдерживаемый метод %s", variant.Method, channel)
			}
			if !seen[channel] {
				channels = append(channels, channel)
				seen[channel] = true
			}
		}
		if variant.Method == "heleket" && seen["crypto"] {
			channels = []string{"crypto"}
		}
		variant.Channels = channels
		clean[id] = variant
	}
	s.PaymentMethodVariants = clean
	// Keep configured positions and append copies absent from an older client.
	ids := []string{}
	for id := range clean {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	order := []string{}
	seen := map[string]bool{}
	for _, id := range append(append(append([]string{}, s.PaymentMethodOrder...), defaultPaymentMethodOrder...), ids...) {
		id = strings.ToLower(strings.TrimSpace(id))
		if s.PaymentMethodBase(id) != "" && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	s.PaymentMethodOrder = order
	return nil
}
