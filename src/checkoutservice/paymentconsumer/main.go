// Separate Lambda binary that subscribes to EventBridge events emitted
// by paymentservice (e.g. PaymentSucceeded, PaymentFailed) and transitions
// the order accordingly. Build this directory as its own deployment.zip.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"checkoutservice/client"
	"checkoutservice/models"
	"checkoutservice/store"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/sirupsen/logrus"
)

// PaymentEventDetail is the EventBridge `detail` payload paymentservice will publish.
type PaymentEventDetail struct {
	OrderID   string `json:"orderId"`
	PaymentID string `json:"paymentId"`
}

var (
	log    *logrus.Logger
	orders store.OrderStore
	cart   *client.CartClient
)

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano}
	log.Out = os.Stdout

	ctx := context.Background()

	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "Orders"
	}
	cartFn := os.Getenv("CART_FUNCTION_NAME")
	if cartFn == "" {
		cartFn = "cartservice"
	}

	var err error
	orders, err = store.NewDynamoOrderStore(ctx, tableName)
	if err != nil {
		log.Fatalf("init order store: %v", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	cart = client.NewCartClient(awslambda.NewFromConfig(cfg), cartFn)
}

func handle(ctx context.Context, evt events.CloudWatchEvent) error {
	var detail PaymentEventDetail
	if err := json.Unmarshal(evt.Detail, &detail); err != nil {
		return fmt.Errorf("unmarshal detail: %w", err)
	}
	if detail.OrderID == "" {
		return fmt.Errorf("missing orderId in event %s", evt.DetailType)
	}

	logger := log.WithFields(logrus.Fields{
		"detailType": evt.DetailType,
		"orderId":    detail.OrderID,
		"paymentId":  detail.PaymentID,
	})

	switch evt.DetailType {
	case "PaymentSucceeded":
		// Conditional update: only flips pending -> paid. Duplicate
		// deliveries are no-ops.
		if err := orders.UpdateStatus(ctx, detail.OrderID, models.StatusPending, models.StatusPaid, detail.PaymentID); err != nil {
			logger.WithError(err).Warn("update to paid failed (likely already transitioned)")
			return nil
		}
		// Empty the cart only after we know payment succeeded.
		order, err := orders.GetOrder(ctx, detail.OrderID)
		if err != nil {
			logger.WithError(err).Error("load order to empty cart")
			return err
		}
		if err := cart.EmptyCart(ctx, order.UserID); err != nil {
			logger.WithError(err).Error("empty cart failed")
			return err
		}
		logger.Info("order paid, cart emptied")
		return nil

	case "PaymentFailed":
		if err := orders.UpdateStatus(ctx, detail.OrderID, models.StatusPending, models.StatusFailed, detail.PaymentID); err != nil {
			logger.WithError(err).Warn("update to failed (likely already transitioned)")
			return nil
		}
		logger.Info("order marked failed")
		return nil

	default:
		logger.Warn("ignoring unknown detail-type")
		return nil
	}
}

func main() {
	lambda.Start(handle)
}
