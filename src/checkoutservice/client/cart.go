package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
)

// CartItem matches the cartservice models.CartItem wire format.
type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type Cart struct {
	UserID string      `json:"user_id"`
	Items  []*CartItem `json:"items"`
}

// cartLambdaEvent matches src/cartservice/main.go CartLambdaEvent.
type cartLambdaEvent struct {
	Action       string                 `json:"action"`
	GetCartReq   map[string]string      `json:"get_cart_req,omitempty"`
	EmptyCartReq map[string]string      `json:"empty_cart_req,omitempty"`
	AddItemReq   map[string]interface{} `json:"add_item_req,omitempty"`
}

type CartClient struct {
	lambda       *awslambda.Client
	functionName string
}

func NewCartClient(lambda *awslambda.Client, functionName string) *CartClient {
	return &CartClient{lambda: lambda, functionName: functionName}
}

func (c *CartClient) GetCart(ctx context.Context, userID string) (*Cart, error) {
	payload, _ := json.Marshal(cartLambdaEvent{
		Action:     "GetCart",
		GetCartReq: map[string]string{"user_id": userID},
	})

	out, err := c.lambda.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName: aws.String(c.functionName),
		Payload:      payload,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke cartservice: %w", err)
	}
	if out.FunctionError != nil {
		return nil, fmt.Errorf("cartservice error: %s", string(out.Payload))
	}

	cart := &Cart{}
	if err := json.Unmarshal(out.Payload, cart); err != nil {
		return nil, fmt.Errorf("unmarshal cart: %w", err)
	}
	return cart, nil
}

func (c *CartClient) EmptyCart(ctx context.Context, userID string) error {
	payload, _ := json.Marshal(cartLambdaEvent{
		Action:       "EmptyCart",
		EmptyCartReq: map[string]string{"user_id": userID},
	})

	out, err := c.lambda.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName: aws.String(c.functionName),
		Payload:      payload,
	})
	if err != nil {
		return fmt.Errorf("invoke cartservice: %w", err)
	}
	if out.FunctionError != nil {
		return fmt.Errorf("cartservice error: %s", string(out.Payload))
	}
	return nil
}
