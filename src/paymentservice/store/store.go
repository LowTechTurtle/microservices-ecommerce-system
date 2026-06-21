package store

import (
	"context"
	"errors"

	"paymentservice/models"
)

var ErrNotFound = errors.New("payment not found")

type PaymentStore interface {
	Create(ctx context.Context, p *models.Payment) error
	GetByID(ctx context.Context, paymentID string) (*models.Payment, error)
	GetByOrderID(ctx context.Context, orderID string) (*models.Payment, error)
	// GetByStripeID is used by the webhook to look up a payment from the
	// Stripe PaymentIntent ID that Stripe sends in every event.
	GetByStripeID(ctx context.Context, stripePaymentIntentID string) (*models.Payment, error)
	// UpdateStatus conditionally transitions status and records metadata.
	// Only succeeds if current status matches expectedFrom (idempotent webhook safety).
	UpdateStatus(ctx context.Context, paymentID string, expectedFrom, to models.PaymentStatus, meta StatusMeta) error
}

type StatusMeta struct {
	StripeRefundID string
	FailureMessage string
}
