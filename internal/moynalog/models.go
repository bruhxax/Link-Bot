package moynalog

import (
	"time"
)

// AuthRequest - структура для запроса аутентификации
type AuthRequest struct {
	Username   string     `json:"username"`
	Password   string     `json:"password"`
	DeviceInfo DeviceInfo `json:"deviceInfo"`
}

// DeviceInfo - информация об устройстве для аутентификации
type DeviceInfo struct {
	SourceDeviceId string      `json:"sourceDeviceId"`
	SourceType     string      `json:"sourceType"`
	AppVersion     string      `json:"appVersion"`
	MetaDetails    MetaDetails `json:"metaDetails"`
}

// MetaDetails - дополнительная информация об устройстве
type MetaDetails struct {
	UserAgent string `json:"userAgent"`
}

// AuthResponse - ответ на запрос аутентификации
type AuthResponse struct {
	Token string `json:"token"`
}

// CreateIncomeRequest - структура для запроса создания дохода (чека)
type CreateIncomeRequest struct {
	OperationTime                   time.Time    `json:"operationTime"`
	RequestTime                     time.Time    `json:"requestTime"`
	Services                        []Service    `json:"services"`
	TotalAmount                     string       `json:"totalAmount"`
	Client                          IncomeClient `json:"client"`
	PaymentType                     string       `json:"paymentType"`
	IgnoreMaxTotalIncomeRestriction bool         `json:"ignoreMaxTotalIncomeRestriction"`
}

// Service - услуга в чеке
type Service struct {
	Name     string  `json:"name"`
	Amount   float64 `json:"amount"`
	Quantity int     `json:"quantity"`
}

// IncomeClient - клиент в чеке
type IncomeClient struct {
	ContactPhone *string `json:"contactPhone,omitempty"`
	DisplayName  *string `json:"displayName,omitempty"`
	INN          *string `json:"inn,omitempty"`
	IncomeType   string  `json:"incomeType"`
}

// CreateIncomeResponse - ответ на запрос создания дохода
type CreateIncomeResponse struct {
	ID            string       `json:"id"`
	OperationTime time.Time    `json:"operationTime"`
	RequestTime   time.Time    `json:"requestTime"`
	Services      []Service    `json:"services"`
	TotalAmount   string       `json:"totalAmount"`
	Client        IncomeClient `json:"client"`
	PaymentType   string       `json:"paymentType"`
	Status        string       `json:"status"`
}
