package main

import (
	"context"
	"fmt"
	"time"

	"paymentservice/events"
	"paymentservice/models"
	"paymentservice/store"
	stripeclient "paymentservice/stripe"

	"github.com/google/uuid"
)

type paymentServer struct {
	payments  store.PaymentStore
	stripe    *stripeclient.Client
	publisher *events.Publisher
}

func (s *paymentServer) createIntent(ctx context.Context, req *models.CreateIntentRequest) (*models.CreateIntentResponse, error) {
	if req.OrderID == "" || req.UserID == "" || req.Currency == "" {
		return nil, fmt.Errorf("orderId, userId, and currency are required")
	}
	if req.PaymentMethod == "" {
		req.PaymentMethod = models.MethodCard
	}

	// Use orderId as idempotency key if none provided — prevents double-charge
	// on Lambda retry for the same order.
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = "checkout-" + req.OrderID
	}

	result, err := s.stripe.CreateIntent(req)
	if err != nil {
		return nil, fmt.Errorf("create stripe intent: %w", err)
	}

	now := time.Now().UTC()
	p := &models.Payment{
		PaymentID:             uuid.NewString(),
		OrderID:               req.OrderID,
		UserID:                req.UserID,
		StripePaymentIntentID: result.StripePaymentIntentID,
		Status:                models.StatusPending,
		Amount:                req.Amount,
		Currency:              req.Currency,
		PaymentMethod:         req.PaymentMethod,
		ClientSecret:          result.ClientSecret,
		QRCodeData:            result.QRCodeData,
		RedirectURL:           result.RedirectURL,
		IdempotencyKey:        req.IdempotencyKey,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	if err := s.payments.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("persist payment: %w", err)
	}

	log.WithField("paymentId", p.PaymentID).
		WithField("method", req.PaymentMethod).
		Info("payment intent created")

	return &models.CreateIntentResponse{
		PaymentID:    p.PaymentID,
		Status:       p.Status,
		ClientSecret: p.ClientSecret,
		QRCodeData:   p.QRCodeData,
		RedirectURL:  p.RedirectURL,
	}, nil
}

func (s *paymentServer) getStatus(ctx context.Context, req *models.GetStatusRequest) (*models.Payment, error) {
	if req.PaymentID == "" {
		return nil, fmt.Errorf("paymentId is required")
	}
	return s.payments.GetByID(ctx, req.PaymentID)
}

func (s *paymentServer) getStatusByOrder(ctx context.Context, req *models.GetStatusByOrderRequest) (*models.Payment, error) {
	if req.OrderID == "" {
		return nil, fmt.Errorf("orderId is required")
	}
	return s.payments.GetByOrderID(ctx, req.OrderID)
}

func (s *paymentServer) refund(ctx context.Context, req *models.RefundRequest) (*models.RefundResponse, error) {
	if req.PaymentID == "" {
		return nil, fmt.Errorf("paymentId is required")
	}

	p, err := s.payments.GetByID(ctx, req.PaymentID)
	if err != nil {
		return nil, fmt.Errorf("load payment: %w", err)
	}
	if p.Status != models.StatusSucceeded {
		return nil, fmt.Errorf("can only refund succeeded payments, current status: %s", p.Status)
	}

	result, err := s.stripe.Refund(p.StripePaymentIntentID, req.AmountMinorUnits)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
	}

	if err := s.payments.UpdateStatus(ctx, p.PaymentID, models.StatusSucceeded, models.StatusRefunded, store.StatusMeta{
		StripeRefundID: result.StripeRefundID,
	}); err != nil {
		log.WithError(err).WithField("paymentId", p.PaymentID).Error("update refund status failed")
	}

	log.WithField("paymentId", p.PaymentID).
		WithField("refundId", result.StripeRefundID).
		Info("payment refunded")

	return &models.RefundResponse{
		PaymentID:      p.PaymentID,
		StripeRefundID: result.StripeRefundID,
		Status:         models.StatusRefunded,
	}, nil
}
