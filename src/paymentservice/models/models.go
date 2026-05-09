package models

import "time"

type PaymentStatus string

const (
	StatusPending    PaymentStatus = "pending"
	StatusProcessing PaymentStatus = "processing"
	StatusSucceeded  PaymentStatus = "succeeded"
	StatusFailed     PaymentStatus = "failed"
	StatusRefunded   PaymentStatus = "refunded"
)

type Money struct {
	CurrencyCode string `json:"currencyCode"`
	Units        int64  `json:"units"`
	Nanos        int32  `json:"nanos"`
}

// MoneyToStripeAmount converts Money to Stripe's integer minor-unit amount.
// Stripe expects cents for USD (e.g. $10.50 → 1050).
func MoneyToStripeAmount(m Money) int64 {
	return m.Units*100 + int64(m.Nanos/10_000_000)
}

// PaymentMethod represents which Stripe payment method types to enable.
// "card"       — standard card (no QR)
// "wechat_pay" — WeChat Pay; Stripe returns a QR code string in next_action
// "alipay"     — Alipay; Stripe returns a redirect URL in next_action
// "promptpay"  — PromptPay (Thailand); returns a QR code
type PaymentMethod string

const (
	MethodCard      PaymentMethod = "card"
	MethodWechatPay PaymentMethod = "wechat_pay"
	MethodAlipay    PaymentMethod = "alipay"
	MethodPromptPay PaymentMethod = "promptpay"
)

type Payment struct {
	PaymentID              string        `json:"paymentId"              dynamodbav:"paymentId"`
	OrderID                string        `json:"orderId"                dynamodbav:"orderId"`
	UserID                 string        `json:"userId"                 dynamodbav:"userId"`
	StripePaymentIntentID  string        `json:"stripePaymentIntentId"  dynamodbav:"stripePaymentIntentId"`
	Status                 PaymentStatus `json:"status"                 dynamodbav:"status"`
	Amount                 Money         `json:"amount"                 dynamodbav:"amount"`
	Currency               string        `json:"currency"               dynamodbav:"currency"`
	PaymentMethod          PaymentMethod `json:"paymentMethod"          dynamodbav:"paymentMethod"`
	// ClientSecret is returned to the frontend for card payments (Stripe Elements).
	ClientSecret           string        `json:"clientSecret,omitempty" dynamodbav:"clientSecret"`
	// QRCodeData is the raw string the frontend should encode into a QR image.
	// Populated for wechat_pay and promptpay intents.
	QRCodeData             string        `json:"qrCodeData,omitempty"   dynamodbav:"qrCodeData"`
	// RedirectURL is used for alipay; the user opens this URL to pay.
	RedirectURL            string        `json:"redirectUrl,omitempty"  dynamodbav:"redirectUrl"`
	StripeRefundID         string        `json:"stripeRefundId,omitempty" dynamodbav:"stripeRefundId"`
	IdempotencyKey         string        `json:"-"                      dynamodbav:"idempotencyKey"`
	FailureMessage         string        `json:"failureMessage,omitempty" dynamodbav:"failureMessage"`
	CreatedAt              time.Time     `json:"createdAt"              dynamodbav:"createdAt"`
	UpdatedAt              time.Time     `json:"updatedAt"              dynamodbav:"updatedAt"`
}

// --- Request / response types for the action-style Lambda event ---

type CreateIntentRequest struct {
	OrderID        string        `json:"orderId"`
	UserID         string        `json:"userId"`
	Amount         Money         `json:"amount"`
	Currency       string        `json:"currency"`
	PaymentMethod  PaymentMethod `json:"paymentMethod"` // defaults to "card" if empty
	IdempotencyKey string        `json:"idempotencyKey"`
}

type CreateIntentResponse struct {
	PaymentID    string        `json:"paymentId"`
	Status       PaymentStatus `json:"status"`
	ClientSecret string        `json:"clientSecret,omitempty"`
	QRCodeData   string        `json:"qrCodeData,omitempty"`
	RedirectURL  string        `json:"redirectUrl,omitempty"`
}

type GetStatusRequest struct {
	PaymentID string `json:"paymentId"`
}

type GetStatusByOrderRequest struct {
	OrderID string `json:"orderId"`
}

type RefundRequest struct {
	PaymentID string `json:"paymentId"`
	// AmountMinorUnits is optional; if zero the full amount is refunded.
	AmountMinorUnits int64 `json:"amountMinorUnits,omitempty"`
}

type RefundResponse struct {
	PaymentID      string        `json:"paymentId"`
	StripeRefundID string        `json:"stripeRefundId"`
	Status         PaymentStatus `json:"status"`
}
