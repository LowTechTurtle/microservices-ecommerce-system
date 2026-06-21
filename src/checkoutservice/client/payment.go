package client

import (
	"context"
	"encoding/json"
	"fmt"

	"checkoutservice/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
)

// CreatePaymentIntent matches the contract paymentservice will expose.
// Until paymentservice exists, NewStubPaymentClient can be used to
// short-circuit this with a fake clientSecret.
type createIntentRequest struct {
	Action string `json:"action"`
	Body   struct {
		OrderID  string       `json:"orderId"`
		UserID   string       `json:"userId"`
		Amount   models.Money `json:"amount"`
		Currency string       `json:"currency"`
	} `json:"body"`
}

type PaymentIntent struct {
	PaymentID    string `json:"paymentId"`
	ClientSecret string `json:"clientSecret"`
}

type PaymentClient interface {
	CreateIntent(ctx context.Context, orderID, userID string, amount models.Money, currency string) (*PaymentIntent, error)
}

type lambdaPaymentClient struct {
	lambda       *awslambda.Client
	functionName string
}

func NewLambdaPaymentClient(lambda *awslambda.Client, functionName string) PaymentClient {
	return &lambdaPaymentClient{lambda: lambda, functionName: functionName}
}

func (c *lambdaPaymentClient) CreateIntent(ctx context.Context, orderID, userID string, amount models.Money, currency string) (*PaymentIntent, error) {
	req := createIntentRequest{Action: "CreatePaymentIntent"}
	req.Body.OrderID = orderID
	req.Body.UserID = userID
	req.Body.Amount = amount
	req.Body.Currency = currency
	payload, _ := json.Marshal(req)

	out, err := c.lambda.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName: aws.String(c.functionName),
		Payload:      payload,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke payment: %w", err)
	}
	if out.FunctionError != nil {
		return nil, fmt.Errorf("payment error: %s", string(out.Payload))
	}

	intent := &PaymentIntent{}
	if err := json.Unmarshal(out.Payload, intent); err != nil {
		return nil, fmt.Errorf("unmarshal payment intent: %w", err)
	}
	return intent, nil
}

// stubPaymentClient lets us build and test checkout end-to-end before
// paymentservice exists.
type stubPaymentClient struct{}

func NewStubPaymentClient() PaymentClient { return &stubPaymentClient{} }

func (s *stubPaymentClient) CreateIntent(ctx context.Context, orderID, userID string, amount models.Money, currency string) (*PaymentIntent, error) {
	return &PaymentIntent{
		PaymentID:    "stub_pi_" + orderID,
		ClientSecret: "stub_cs_" + orderID,
	}, nil
}
