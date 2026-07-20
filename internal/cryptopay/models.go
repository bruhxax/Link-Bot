package cryptopay

import "time"

type InvoiceRequest struct {
	CurrencyType   string `json:"currency_type,omitempty"`
	Asset          string `json:"asset,omitempty"`
	Fiat           string `json:"fiat,omitempty"`
	AcceptedAssets string `json:"accepted_assets,omitempty"`
	Amount         string `json:"amount,omitempty"`
	Description    string `json:"description,omitempty"`
	HiddenMessage  string `json:"hidden_message,omitempty"`
	PaidBtnName    string `json:"paid_btn_name,omitempty"`
	PaidBtnUrl     string `json:"paid_btn_url,omitempty"`
	Payload        string `json:"payload,omitempty"`
	AllowComments  *bool  `json:"allow_comments,omitempty"`
	AllowAnonymous *bool  `json:"allow_anonymous,omitempty"`
	ExpiresIn      *int   `json:"expires_in,omitempty"`
}

type InvoiceResponse struct {
	InvoiceID         *int64     `json:"invoice_id"`
	Hash              string     `json:"hash"`
	CurrencyType      string     `json:"currency_type"`
	Asset             string     `json:"asset"`
	Fiat              string     `json:"fiat"`
	Amount            string     `json:"amount"`
	PaidAsset         string     `json:"paid_asset"`
	PaidAmount        string     `json:"paid_amount"`
	PaidFiatRate      string     `json:"paid_fiat_rate"`
	AcceptedAssets    []string   `json:"accepted_assets"`
	FeeAsset          string     `json:"fee_asset"`
	FeeAmount         *string    `json:"fee_amount"`
	BotInvoiceUrl     string     `json:"bot_invoice_url"`
	MiniAppInvoiceUrl string     `json:"mini_app_invoice_url"`
	WebAppInvoiceUrl  string     `json:"web_app_invoice_url"`
	Description       string     `json:"description"`
	Status            string     `json:"status"`
	CreatedAt         *time.Time `json:"created_at"`
	PaidUsdRate       string     `json:"paid_usd_rate"`
	AllowComments     bool       `json:"allow_comments"`
	AllowAnonymous    bool       `json:"allow_anonymous"`
	ExpirationDate    *time.Time `json:"expiration_date"`
	PaidAt            *time.Time `json:"paid_at"`
	PaidAnonymously   bool       `json:"paid_anonymously"`
	Comment           string     `json:"comment"`
	HiddenMessage     string     `json:"hidden_message"`
	Payload           string     `json:"payload"`
	PaidBtnName       string     `json:"paid_btn_name"`
	PaidBtnUrl        string     `json:"paid_btn_url"`
	PayUrl            string     `json:"pay_url"`
}

func (r InvoiceResponse) IsPaid() bool {
	return r.Status == "paid"
}

type ResponseWrapper[T any] struct {
	Ok     bool `json:"ok"`
	Result T    `json:"result"`
}

type ResultListWrapper[T any] struct {
	Items []T `json:"items"`
}

type ResponseListWrapper[T any] struct {
	Ok     bool                 `json:"ok"`
	Result ResultListWrapper[T] `json:"result"`
}
