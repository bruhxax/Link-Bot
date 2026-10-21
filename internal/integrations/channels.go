package integrations

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"link-bot/internal/paymentchannel"
)

func (g *Gateway) PaymentChannels(ctx context.Context, method string) (paymentchannel.Catalog, error) {
	catalog := paymentchannel.For(method)
	provider := paymentchannel.Provider(method)
	cfg, ok := g.settings.Config(provider)
	if !ok {
		return catalog, fmt.Errorf("платёжная система выключена или не настроена")
	}
	if provider == ProviderSmerch {
		method := strings.TrimSpace(cfg["paymentMethod"])
		if paymentchannel.Valid(provider, method) && method != "sbp" && method != "crypto" {
			catalog.Options = append(catalog.Options, paymentchannel.Option{ID: method, Name: method + " · метод интеграции"})
		}
		catalog.Note = "Методы должны быть подключены к кассе с RUB и ссылкой оплаты. Другой код метода можно указать в интеграции Smerch."
		return catalog, nil
	}
	if provider == ProviderLava {
		return g.lavaPaymentChannels(ctx, cfg, catalog)
	}
	if provider == ProviderFreeKassa || provider == ProviderKassaAI {
		if cfg["apiKey"] == "" {
			catalog.Options = []paymentchannel.Option{}
			catalog.Note = "Для выбора конкретных методов FreeKassa добавьте API-ключ в интеграции. Обычная форма позволяет покупателю менять способ."
			return catalog, nil
		}
		return g.freeKassaPaymentChannels(ctx, cfg, catalog)
	}
	if provider != ProviderHeleket {
		return catalog, nil
	}
	raw := []byte(`{}`)
	sign := fmt.Sprintf("%x", md5.Sum([]byte(base64.StdEncoding.EncodeToString(raw)+cfg["apiKey"])))
	var response struct {
		State  int `json:"state"`
		Result []struct {
			Currency  string `json:"currency"`
			Network   string `json:"network"`
			Available bool   `json:"is_available"`
		} `json:"result"`
	}
	endpoint := strings.TrimRight(cfg["apiUrl"], "/") + "/v1/payment/services"
	if err := g.doJSON(ctx, http.MethodPost, endpoint, raw, map[string]string{"merchant": cfg["merchantId"], "sign": sign}, &response); err != nil {
		return catalog, err
	}
	if response.State != 0 {
		return catalog, fmt.Errorf("не удалось получить методы Heleket")
	}
	options := []paymentchannel.Option{}
	seen := map[string]bool{}
	for _, item := range response.Result {
		if !item.Available {
			continue
		}
		id := strings.ToUpper(item.Currency)
		if item.Network != "" {
			id += ":" + item.Network
		}
		if !paymentchannel.Valid(method, id) || seen[id] {
			continue
		}
		seen[id] = true
		options = append(options, paymentchannel.Option{ID: id, Name: paymentchannel.Label(method, id)})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Name < options[j].Name })
	catalog.Options = append(catalog.Options, options...)
	catalog.Live = true
	return catalog, nil
}

func channelConfig(cfg map[string]string, input CreatePaymentRequest) map[string]string {
	// Config returns a copy, but keep this helper independent of that guarantee.
	copy := make(map[string]string, len(cfg))
	for k, v := range cfg {
		copy[k] = v
	}
	if len(input.Channels) > 0 {
		switch input.Provider {
		case ProviderCisPay, ProviderPayHot, ProviderSmerch:
			copy["paymentMethod"] = input.Channels[0]
		case ProviderKassaAI:
			copy["paymentSystemId"] = input.Channels[0]
		}
	}
	return copy
}

func applyCheckoutChannels(payload map[string]any, input CreatePaymentRequest) {
	if len(input.Channels) == 0 {
		return
	}
	switch input.Provider {
	case ProviderAuraPay, ProviderParityPay:
		payload["service"] = input.Channels[0]
	case ProviderAntiloPay:
		payload["prefer_methods"] = input.Channels
	case ProviderTribute:
		methods := []map[string]string{}
		for _, channel := range input.Channels {
			methods = append(methods, map[string]string{"type": channel})
		}
		payload["preferredPaymentMethods"] = methods
	case ProviderPlatega:
		// Platega expects an integer rather than the display code.
		var code int
		_, _ = fmt.Sscan(input.Channels[0], &code)
		payload["paymentMethod"] = code
	case ProviderRollyPay:
		payload["payment_method"] = input.Channels[0]
	case ProviderAnore:
		payload["methods"] = input.Channels
	case ProviderHeleket:
		currencies := []map[string]string{}
		for _, channel := range input.Channels {
			if channel == "crypto" {
				return
			}
			parts := strings.SplitN(channel, ":", 2)
			item := map[string]string{"currency": parts[0]}
			if len(parts) == 2 {
				item["network"] = parts[1]
			}
			currencies = append(currencies, item)
		}
		payload["currencies"] = currencies
	}
}

func marshalCheckoutPayload(input CreatePaymentRequest, payload map[string]any) ([]byte, error) {
	applyCheckoutChannels(payload, input)
	return json.Marshal(payload)
}

func (g *Gateway) lavaPaymentChannels(ctx context.Context, cfg map[string]string, catalog paymentchannel.Catalog) (paymentchannel.Catalog, error) {
	raw, _ := json.Marshal(map[string]string{"shopId": cfg["shopId"]})
	var response struct {
		Status int `json:"status"`
		Data   []struct {
			ID     string `json:"service_id"`
			Name   string `json:"service_name"`
			Status int    `json:"status"`
		} `json:"data"`
	}
	if err := g.doJSON(ctx, http.MethodPost, "https://api.lava.ru/business/invoice/get-available-tariffs", raw, map[string]string{"Signature": hmacHex(sha256.New, []byte(cfg["secretKey"]), raw)}, &response); err != nil {
		return catalog, err
	}
	if response.Status != 200 {
		return catalog, fmt.Errorf("LAVA не вернула методы кассы")
	}
	catalog.Options = []paymentchannel.Option{}
	catalog.Live = true
	for _, item := range response.Data {
		if item.Status == 200 && paymentchannel.Valid("lava", item.ID) {
			catalog.Options = append(catalog.Options, paymentchannel.Option{ID: item.ID, Name: firstNonEmpty(item.Name, item.ID)})
		}
	}
	return catalog, nil
}

func (g *Gateway) freeKassaPaymentChannels(ctx context.Context, cfg map[string]string, catalog paymentchannel.Catalog) (paymentchannel.Catalog, error) {
	shop, err := strconv.ParseInt(cfg["shopId"], 10, 64)
	if err != nil {
		return catalog, err
	}
	params := map[string]any{"shopId": shop, "nonce": nextKassaAINonce()}
	keys := []string{"nonce", "shopId"}
	values := []string{}
	for _, key := range keys {
		values = append(values, fmt.Sprint(params[key]))
	}
	params["signature"] = hmacHex(sha256.New, []byte(cfg["apiKey"]), []byte(strings.Join(values, "|")))
	raw, _ := json.Marshal(params)
	var response struct {
		Type       string `json:"type"`
		Currencies []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			Currency string `json:"currency"`
			Enabled  int    `json:"is_enabled"`
		} `json:"currencies"`
	}
	if err := g.doJSON(ctx, http.MethodPost, "https://api.fk.life/v1/currencies", raw, nil, &response); err != nil {
		return catalog, err
	}
	if response.Type != "success" {
		return catalog, fmt.Errorf("FreeKassa не вернула методы кассы")
	}
	catalog.Options = []paymentchannel.Option{}
	catalog.Live = true
	for _, item := range response.Currencies {
		if item.Enabled == 1 {
			catalog.Options = append(catalog.Options, paymentchannel.Option{ID: strconv.Itoa(item.ID), Name: strings.TrimSpace(item.Name + " " + item.Currency)})
		}
	}
	return catalog, nil
}
