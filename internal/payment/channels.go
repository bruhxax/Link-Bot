package payment

import (
	"context"
	"fmt"
	"strings"

	"link-bot/internal/paymentchannel"
)

// CheckoutChannels applies the integration defaults to unrestricted Smerch
// rows and checks explicit copies against the currently enabled methods.
func (s PaymentService) CheckoutChannels(ctx context.Context, method string, explicit []string) ([]string, error) {
	if paymentchannel.Provider(method) != "smerch" {
		return explicit, nil
	}
	catalog, err := s.PaymentChannels(ctx, method)
	if err != nil {
		return nil, err
	}
	if len(explicit) == 0 {
		return append([]string(nil), catalog.DefaultChannels...), nil
	}
	for _, channel := range explicit {
		found := false
		for _, option := range catalog.Options {
			found = found || option.ID == channel
		}
		if !found {
			return nil, fmt.Errorf("Smerch: способ оплаты изменился; выберите его снова")
		}
	}
	return explicit, nil
}

func firstPaymentChannel(channels []string) string {
	if len(channels) > 0 {
		return channels[0]
	}
	return ""
}

func (s PaymentService) PaymentChannels(ctx context.Context, method string) (paymentchannel.Catalog, error) {
	catalog := paymentchannel.For(method)
	switch paymentchannel.Provider(method) {
	case "yookassa":
		client := s.currentYookasaClient()
		if client == nil {
			return catalog, nil
		}
		methods, err := client.AvailablePaymentMethods(ctx)
		if err != nil {
			return catalog, err
		}
		catalog.Options = []paymentchannel.Option{}
		for _, id := range methods {
			if paymentchannel.Valid(method, id) {
				catalog.Options = append(catalog.Options, paymentchannel.Option{ID: id, Name: paymentchannel.Label(method, id)})
			}
		}
		catalog.Live = true
		catalog.Note = "Способы, подключённые в вашей кассе ЮKassa."
		return catalog, nil
	case "cryptopay":
		_, assets := s.currentCryptoPayClient()
		catalog.Options = []paymentchannel.Option{}
		for _, id := range strings.Split(assets, ",") {
			id = strings.TrimSpace(id)
			if paymentchannel.Valid(method, id) {
				catalog.Options = append(catalog.Options, paymentchannel.Option{ID: id, Name: id})
			}
		}
		return catalog, nil
	default:
		if s.integrationGateway != nil {
			return s.integrationGateway.PaymentChannels(ctx, method)
		}
	}
	return catalog, nil
}
