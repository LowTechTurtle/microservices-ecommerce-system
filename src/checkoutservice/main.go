package main

import (
	"context"
	"os"
	"time"

	"checkoutservice/client"
	"checkoutservice/store"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx := context.Background()

	tableName := envOr("DYNAMODB_TABLE_NAME", "Orders")
	cartFn := envOr("CART_FUNCTION_NAME", "cartservice")
	catalogFn := envOr("CATALOG_FUNCTION_NAME", "productcatalogservice")
	paymentFn := os.Getenv("PAYMENT_FUNCTION_NAME") // empty = use stub

	orders, err := store.NewDynamoOrderStore(ctx, tableName)
	if err != nil {
		log.Fatalf("init order store: %v", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	lambdaClient := awslambda.NewFromConfig(cfg)

	var paymentClient client.PaymentClient
	if paymentFn == "" {
		log.Warn("PAYMENT_FUNCTION_NAME unset — using stub payment client")
		paymentClient = client.NewStubPaymentClient()
	} else {
		paymentClient = client.NewLambdaPaymentClient(lambdaClient, paymentFn)
	}

	srv := &checkoutServer{
		orders:  orders,
		cart:    client.NewCartClient(lambdaClient, cartFn),
		catalog: client.NewCatalogClient(lambdaClient, catalogFn),
		payment: paymentClient,
	}

	log.Info("checkoutservice starting")
	lambda.Start(srv.handler)
}
