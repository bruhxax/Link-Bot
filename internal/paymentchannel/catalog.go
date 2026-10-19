// Package paymentchannel describes channels that the connected checkout APIs
// can actually enforce. A provider's hosted page is never scraped or modified.
package paymentchannel

import (
	"fmt"
	"regexp"
	"strings"
)

type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Catalog struct {
	Options      []Option `json:"options"`
	Multiple     bool     `json:"multiple"`
	Live         bool     `json:"live"`
	Note         string   `json:"note,omitempty"`
	Preference   bool     `json:"preference,omitempty"`
	DefaultLabel string   `json:"defaultLabel,omitempty"`
}

func Base(id string) string { return strings.SplitN(id, "__", 2)[0] }

func Provider(id string) string {
	switch id = Base(id); id {
	case "sbp", "card":
		return "yookassa"
	case "crypto":
		return "cryptopay"
	default:
		return id
	}
}

func For(method string) Catalog {
	c := Catalog{Options: []Option{}}
	switch Provider(method) {
	case "lava":
		c.Multiple = true
		c.Options = []Option{{"card", "Банковская карта"}, {"sbp", "СБП"}, {"sber_pay", "SberPay"}, {"mir_pay", "Mir Pay"}, {"mir_card", "Карта Мир"}, {"lava_pay_in", "Кошелёк LAVA"}}
	case "paritypay", "aurapay":
		c.Options = []Option{{"card", "Банковская карта"}, {"sbp", "СБП"}}
		if Provider(method) == "aurapay" {
			c.Preference = true
		}
	case "antilopay":
		c.Multiple = true
		c.Preference = true
		c.Options = []Option{{"CARD_RU", "Банковская карта"}, {"SBP", "СБП"}, {"SBER_PAY", "SberPay"}}
	case "tribute":
		c.Multiple = true
		c.Preference = true
		c.Options = []Option{{"card", "Банковская карта"}, {"sbp", "СБП"}, {"wallet_pay", "Wallet Pay"}, {"apple_pay", "Apple Pay"}}
	case "yookassa":
		c.Options = []Option{{"bank_card", "Банковская карта"}, {"sbp", "СБП"}, {"sberbank", "SberPay"}, {"yoo_money", "ЮMoney"}, {"alfa_pay", "Alfa Pay"}, {"tinkoff_bank", "T-Pay"}, {"sber_loan", "Покупки в кредит"}, {"sber_bnpl", "Плати частями"}}
	case "platega":
		c.Options = []Option{{"2", "СБП / SberPay"}, {"11", "Карты РФ"}, {"12", "Международные карты"}, {"13", "Криптовалюта"}}
	case "heleket":
		c.Multiple = true
		c.Options = []Option{{"crypto", "Криптовалюта · все монеты"}}
	case "cryptopay":
		c.Multiple = true
		for _, asset := range []string{"USDT", "TON", "BTC", "ETH", "LTC", "BNB", "TRX", "USDC"} {
			c.Options = append(c.Options, Option{asset, asset})
		}
	case "rollypay":
		c.Options = []Option{{"sbp", "СБП"}, {"card", "Карты РФ"}, {"intl_card", "Международные карты"}, {"crypto", "Криптовалюта"}}
	case "pally":
		c.Options = []Option{{"BANK_CARD", "Банковская карта"}, {"SBP", "СБП"}}
	case "cispay":
		c.Options = []Option{{"SBP", "СБП"}, {"CARD", "Банковская карта"}}
	case "anore":
		c.Multiple = true
		c.Options = []Option{{"sbp", "СБП"}, {"card", "Банковская карта"}, {"yoomoney", "ЮMoney"}, {"crypto", "Криптовалюта"}}
	case "kassaai":
		c.Options = []Option{{"44", "СБП"}, {"36", "Карты РФ"}, {"43", "SberPay"}}
	case "payhot":
		c.Options = []Option{{"sbp", "СБП"}, {"card", "Банковская карта"}, {"sberpay", "SberPay"}, {"crypto", "Криптовалюта"}, {"applepay", "Apple Pay"}, {"googlepay", "Google Pay"}}
	case "freekassa":
		c.Options = []Option{{"36", "Карты РФ (API)"}, {"44", "СБП (API)"}, {"43", "SberPay"}, {"4", "VISA RUB"}, {"8", "Mastercard RUB"}, {"12", "Мир"}, {"6", "ЮMoney"}, {"14", "USDT · ERC20"}, {"15", "USDT · TRC20"}, {"24", "Bitcoin"}, {"26", "Ethereum"}, {"39", "Tron"}, {"42", "СБП"}}
	case "wata":
		c.Note = "API платёжных ссылок WATA не принимает ограничение способов. Их набор задаётся терминалом в кабинете WATA."
	case "mulenpay":
		c.Note = "API создания платежа MulenPay не принимает список методов. Доступные карты и СБП настраиваются в кассе MulenPay."
	case "datagio":
		c.Note = "API платёжных ссылок Datagio не принимает ограничение методов. Выбор СБП и криптовалюты остаётся на стороне Datagio."
	case "cloudpayments":
		c.Note = "Orders API CloudPayments не предоставляет фильтр методов для отдельной ссылки. Способы настраиваются в кабинете CloudPayments."
	case "balance", "stars", "p2p":
		c.Note = "У этого способа оплаты нет дополнительных методов."
	default:
		c.Note = "Подключённый API не предоставляет подтверждённого ограничения методов. Используется выбор на стороне платёжной системы."
	}
	if len(c.Options) > 0 {
		c.Note = "Доступность способов зависит от подключения в вашей кассе."
	}
	if c.Preference {
		c.Note = "API задаёт предпочтительный способ, но не гарантирует скрытие остальных: покупатель может изменить его на странице оплаты."
	}
	if Provider(method) == "cispay" || Provider(method) == "kassaai" || Provider(method) == "payhot" {
		c.DefaultLabel = "Как в интеграции"
	}
	return c
}

var coinPattern = regexp.MustCompile(`^[A-Z0-9]{2,16}(:[a-zA-Z0-9_-]{1,32})?$`)
var servicePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
var currencyIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,5}$`)

func Valid(method, channel string) bool {
	for _, option := range For(method).Options {
		if option.ID == channel {
			return true
		}
	}
	return Provider(method) == "heleket" && coinPattern.MatchString(channel) || Provider(method) == "lava" && servicePattern.MatchString(channel) || (Provider(method) == "freekassa" || Provider(method) == "kassaai") && currencyIDPattern.MatchString(channel)
}

func Label(method, channel string) string {
	for _, option := range For(method).Options {
		if option.ID == channel {
			return option.Name
		}
	}
	return strings.ReplaceAll(channel, ":", " · ")
}

// Resolve prevents a caller from escaping a row's allowed channels. An empty
// selection retains the old provider behaviour when the row is unrestricted.
func Resolve(method string, allowed []string, selected string) ([]string, error) {
	if len(allowed) == 0 {
		if selected != "" {
			return nil, fmt.Errorf("способ оплаты изменился; выберите его снова")
		}
		return nil, nil
	}
	for _, channel := range allowed {
		if !Valid(method, channel) {
			return nil, fmt.Errorf("недопустимый метод оплаты: %s", channel)
		}
	}
	if selected != "" {
		for _, channel := range allowed {
			if channel == selected {
				return []string{channel}, nil
			}
		}
		return nil, fmt.Errorf("этот метод не разрешён для выбранной платёжки")
	}
	if len(allowed) == 1 || For(method).Multiple {
		return append([]string(nil), allowed...), nil
	}
	return nil, fmt.Errorf("выберите конкретный метод оплаты")
}
