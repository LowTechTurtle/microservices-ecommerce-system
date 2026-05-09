package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"paymentservice/events"
	"paymentservice/models"
	"paymentservice/store"
	stripeclient "paymentservice/stripe"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
}

// PaymentLambdaEvent is the action-style event that checkoutservice sends
// via Lambda Invoke — matching the cartservice pattern.
type PaymentLambdaEvent struct {
	Action              string                          `json:"action"`
	CreateIntentReq     *models.CreateIntentRequest     `json:"create_intent_req,omitempty"`
	GetStatusReq        *models.GetStatusRequest        `json:"get_status_req,omitempty"`
	GetStatusByOrderReq *models.GetStatusByOrderRequest `json:"get_status_by_order_req,omitempty"`
	RefundReq           *models.RefundRequest           `json:"refund_req,omitempty"`
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx := context.Background()

	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeKey == "" {
		log.Fatal("STRIPE_SECRET_KEY must be set")
	}

	tableName := envOr("DYNAMODB_TABLE_NAME", "Payments")
	busName := envOr("EVENT_BUS_NAME", "default")

	payments, err := store.NewDynamoPaymentStore(ctx, tableName)
	if err != nil {
		log.Fatalf("init payment store: %v", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	srv := &paymentServer{
		payments:  payments,
		stripe:    stripeclient.NewClient(stripeKey),
		publisher: events.NewPublisher(eventbridge.NewFromConfig(cfg), busName),
	}

	log.Info("paymentservice starting")
	lambda.Start(srv.handle)
}

func (s *paymentServer) handle(ctx context.Context, evt PaymentLambdaEvent) (interface{}, error) {
	switch evt.Action {
	case "CreatePaymentIntent":
		if evt.CreateIntentReq == nil {
			return nil, fmt.Errorf("missing create_intent_req")
		}
		return s.createIntent(ctx, evt.CreateIntentReq)

	case "GetPaymentStatus":
		if evt.GetStatusReq == nil {
			return nil, fmt.Errorf("missing get_status_req")
		}
		p, err := s.getStatus(ctx, evt.GetStatusReq)
		if err == store.ErrNotFound {
			return nil, fmt.Errorf("payment not found")
		}
		return p, err

	case "GetPaymentStatusByOrder":
		if evt.GetStatusByOrderReq == nil {
			return nil, fmt.Errorf("missing get_status_by_order_req")
		}
		p, err := s.getStatusByOrder(ctx, evt.GetStatusByOrderReq)
		if err == store.ErrNotFound {
			return nil, fmt.Errorf("payment not found for order")
		}
		return p, err

	case "RefundPayment":
		if evt.RefundReq == nil {
			return nil, fmt.Errorf("missing refund_req")
		}
		return s.refund(ctx, evt.RefundReq)

	default:
		return nil, fmt.Errorf("unknown action: %s", evt.Action)
	}
}

// handleWebhookEvent is called by the webhook Lambda (webhook/main.go) after
// Stripe signature verification. It updates DynamoDB and fires EventBridge.
func (s *paymentServer) handleWebhookEvent(ctx context.Context, eventType string, piID string, failureMsg string) error {
	p, err := s.payments.GetByStripeID(ctx, piID)
	if err != nil {
		return fmt.Errorf("lookup payment by stripe id %s: %w", piID, err)
	}

	logger := log.WithField("paymentId", p.PaymentID).WithField("stripeEvent", eventType)

	switch eventType {
	case "payment_intent.succeeded":
		if err := s.payments.UpdateStatus(ctx, p.PaymentID, models.StatusPending, models.StatusSucceeded, store.StatusMeta{}); err != nil {
			logger.WithError(err).Warn("status update skipped (likely already transitioned)")
			return nil
		}
		if err := s.publisher.PaymentSucceeded(ctx, p.OrderID, p.PaymentID); err != nil {
			logger.WithError(err).Error("publish PaymentSucceeded failed")
			return err
		}
		logger.Info("payment succeeded")

	case "payment_intent.payment_failed":
		if err := s.payments.UpdateStatus(ctx, p.PaymentID, models.StatusPending, models.StatusFailed, store.StatusMeta{
			FailureMessage: failureMsg,
		}); err != nil {
			logger.WithError(err).Warn("status update skipped (likely already transitioned)")
			return nil
		}
		if err := s.publisher.PaymentFailed(ctx, p.OrderID, p.PaymentID); err != nil {
			logger.WithError(err).Error("publish PaymentFailed failed")
			return err
		}
		logger.Info("payment failed")

	case "charge.refunded":
		// Refunds initiated via our RefundPayment action already update status.
		// This handles external refunds made directly in the Stripe dashboard.
		if err := s.payments.UpdateStatus(ctx, p.PaymentID, models.StatusSucceeded, models.StatusRefunded, store.StatusMeta{}); err != nil {
			logger.WithError(err).Warn("refund status update skipped")
		}
		logger.Info("charge refunded via webhook")

	default:
		// Decode the raw payload for debugging but don't error — Stripe sends
		// many event types we don't need to handle.
		raw, _ := json.Marshal(map[string]string{"type": eventType, "pi": piID})
		logger.WithField("raw", string(raw)).Debug("ignoring unhandled stripe event")
	}

	return nil
}
