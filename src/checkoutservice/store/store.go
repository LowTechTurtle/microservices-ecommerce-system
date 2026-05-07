package store

import (
	"context"

	"checkoutservice/models"
)

type OrderStore interface {
	CreateOrder(ctx context.Context, order *models.Order) error
	GetOrder(ctx context.Context, orderID string) (*models.Order, error)
	// UpdateStatus performs a conditional update: it only succeeds if the
	// current status matches expectedFrom. This makes duplicate webhook
	// deliveries safe to retry without double-applying transitions.
	UpdateStatus(ctx context.Context, orderID string, expectedFrom, to models.OrderStatus, paymentID string) error
}
