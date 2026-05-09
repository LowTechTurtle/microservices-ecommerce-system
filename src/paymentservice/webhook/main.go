// Separate Lambda binary that receives Stripe webhook events via API Gateway.
// Build this directory as its own deployment: webhook.zip
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"paymentservice/events"
	"paymentservice/models"
	"paymentservice/store"
	stripeclient "paymentservice/stripe"

	goevents "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/sirupsen/logrus"
	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/webhook"
)

var (
	log     *logrus.Logger
	h       *webhookHandler
)

type webhookHandler struct {
	payments      store.PaymentStore
	publisher     *events.Publisher
	webhookSecret string
}

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano}
	log.Out = os.Stdout

	ctx := context.Background()

	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeKey == "" || webhookSecret == "" {
		log.Fatal("STRIPE_SECRET_KEY and STRIPE_WEBHOOK_SECRET must be set")
	}

	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "Payments"
	}
	busName := os.Getenv("EVENT_BUS_NAME")
	if busName == "" {
		busName = "default"
	}

	payments, err := store.NewDynamoPaymentStore(ctx, tableName)
	if err != nil {
		log.Fatalf("init payment store: %v", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	// Set the Stripe key for this binary.
	stripeclient.NewClient(stripeKey)

	h = &webhookHandler{
		payments:      payments,
		publisher:     events.NewPublisher(eventbridge.NewFromConfig(cfg), busName),
		webhookSecret: webhookSecret,
	}
}

func handle(ctx context.Context, req goevents.APIGatewayV2HTTPRequest) (goevents.APIGatewayV2HTTPResponse, error) {
	sig := req.Headers["stripe-signature"]
	if sig == "" {
		return errResponse(http.StatusBadRequest, "missing stripe-signature header")
	}

	// ConstructEvent verifies the HMAC signature and rejects replays older than
	// Stripe's default tolerance window (300 s). Raw body must not be decoded
	// before this call or the signature check will fail.
	evt, err := webhook.ConstructEvent([]byte(req.Body), sig, h.webhookSecret)
	if err != nil {
		log.WithError(err).Warn("stripe signature verification failed")
		return errResponse(http.StatusBadRequest, "invalid signature")
	}

	if err := h.dispatch(ctx, evt); err != nil {
		log.WithError(err).WithField("type", evt.Type).Error("webhook dispatch failed")
		// Return 500 so Stripe retries (it retries on non-2xx for up to 3 days).
		return errResponse(http.StatusInternalServerError, "internal error")
	}

	return goevents.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"received":true}`,
	}, nil
}

func (h *webhookHandler) dispatch(ctx context.Context, evt stripe.Event) error {
	logger := log.WithField("eventId", evt.ID).WithField("type", evt.Type)

	switch evt.Type {
	case "payment_intent.succeeded":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(evt.Data.Raw, &pi); err != nil {
			return fmt.Errorf("unmarshal PaymentIntent: %w", err)
		}
		return h.onSucceeded(ctx, logger, pi.ID)

	case "payment_intent.payment_failed":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(evt.Data.Raw, &pi); err != nil {
			return fmt.Errorf("unmarshal PaymentIntent: %w", err)
		}
		failureMsg := ""
		if pi.LastPaymentError != nil {
			failureMsg = pi.LastPaymentError.Msg
		}
		return h.onFailed(ctx, logger, pi.ID, failureMsg)

	case "charge.refunded":
		var ch stripe.Charge
		if err := json.Unmarshal(evt.Data.Raw, &ch); err != nil {
			return fmt.Errorf("unmarshal Charge: %w", err)
		}
		if ch.PaymentIntent == nil {
			logger.Warn("charge.refunded has no PaymentIntent, skipping")
			return nil
		}
		return h.onExternalRefund(ctx, logger, ch.PaymentIntent.ID)

	default:
		logger.Debug("unhandled stripe event type, ignoring")
		return nil
	}
}

func (h *webhookHandler) onSucceeded(ctx context.Context, logger *logrus.Entry, piID string) error {
	p, err := h.lookupByStripeID(ctx, piID)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if err := h.payments.UpdateStatus(ctx, p.PaymentID, models.StatusPending, models.StatusSucceeded, store.StatusMeta{}); err != nil {
		logger.WithError(err).Warn("succeeded update skipped (already transitioned)")
		return nil
	}
	_ = h.publisher.PaymentSucceeded(ctx, p.OrderID, p.PaymentID)
	logger.WithField("paymentId", p.PaymentID).Info("payment succeeded")
	return nil
}

func (h *webhookHandler) onFailed(ctx context.Context, logger *logrus.Entry, piID, failureMsg string) error {
	p, err := h.lookupByStripeID(ctx, piID)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if err := h.payments.UpdateStatus(ctx, p.PaymentID, models.StatusPending, models.StatusFailed, store.StatusMeta{
		FailureMessage: failureMsg,
	}); err != nil {
		logger.WithError(err).Warn("failed update skipped (already transitioned)")
		return nil
	}
	_ = h.publisher.PaymentFailed(ctx, p.OrderID, p.PaymentID)
	logger.WithField("paymentId", p.PaymentID).Info("payment failed")
	return nil
}

func (h *webhookHandler) onExternalRefund(ctx context.Context, logger *logrus.Entry, piID string) error {
	p, err := h.lookupByStripeID(ctx, piID)
	if err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if err := h.payments.UpdateStatus(ctx, p.PaymentID, models.StatusSucceeded, models.StatusRefunded, store.StatusMeta{}); err != nil {
		logger.WithError(err).Warn("external refund update skipped (already transitioned)")
		return nil
	}
	logger.WithField("paymentId", p.PaymentID).Info("external refund recorded")
	return nil
}

func (h *webhookHandler) lookupByStripeID(ctx context.Context, piID string) (*models.Payment, error) {
	p, err := h.payments.GetByStripeID(ctx, piID)
	if err == store.ErrNotFound {
		log.WithField("stripePI", piID).Warn("payment not found for stripe event, skipping")
		return nil, nil
	}
	return p, err
}

func errResponse(code int, msg string) (goevents.APIGatewayV2HTTPResponse, error) {
	body, _ := json.Marshal(map[string]string{"error": msg})
	return goevents.APIGatewayV2HTTPResponse{
		StatusCode: code,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}

func main() {
	lambda.Start(handle)
}
