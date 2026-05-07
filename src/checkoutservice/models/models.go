package models

import "time"

// Money mirrors the productcatalogservice Money type.
type Money struct {
	CurrencyCode string `json:"currencyCode"`
	Units        int64  `json:"units"`
	Nanos        int32  `json:"nanos"`
}

type Address struct {
	StreetAddress string `json:"streetAddress"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	ZipCode       string `json:"zipCode"`
}

// OrderItem snapshots the product price at order-creation time.
type OrderItem struct {
	ProductID string `json:"productId"`
	Name      string `json:"name"`
	Quantity  int32  `json:"quantity"`
	UnitPrice Money  `json:"unitPrice"`
	LineTotal Money  `json:"lineTotal"`
}

type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusPaid      OrderStatus = "paid"
	StatusFailed    OrderStatus = "failed"
	StatusFulfilled OrderStatus = "fulfilled"
	StatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	OrderID         string      `json:"orderId"`
	UserID          string      `json:"userId"`
	Status          OrderStatus `json:"status"`
	Items           []OrderItem `json:"items"`
	Subtotal        Money       `json:"subtotal"`
	Tax             Money       `json:"tax"`
	Shipping        Money       `json:"shipping"`
	Total           Money       `json:"total"`
	Currency        string      `json:"currency"`
	ShippingAddress Address     `json:"shippingAddress"`
	PaymentID       string      `json:"paymentId,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

// CheckoutRequest is the public POST /checkout payload.
type CheckoutRequest struct {
	UserID          string  `json:"userId"`
	Currency        string  `json:"currency"`
	ShippingAddress Address `json:"shippingAddress"`
}

// CheckoutResponse is returned to the client; clientSecret is what
// Stripe Elements uses to confirm payment from the browser.
type CheckoutResponse struct {
	OrderID      string `json:"orderId"`
	ClientSecret string `json:"clientSecret"`
	Total        Money  `json:"total"`
}
