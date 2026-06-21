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

type CartClient struct {
	lambda       *awslambda.Client
	functionName string
}

func NewCartClient(lambda *awslambda.Client, functionName string) *CartClient {
	return &CartClient{lambda: lambda, functionName: functionName}
}

func (c *CartClient) GetCart(ctx context.Context, userID string) (*Cart, error) {
	// Build API Gateway V2 HTTP request event
	event := map[string]interface{}{
		"rawPath": "/cart",
		"requestContext": map[string]interface{}{
			"http": map[string]interface{}{
				"method": "GET",
			},
		},
		"queryStringParameters": map[string]string{
			"user_id": userID,
		},
	}
	
	payload, _ := json.Marshal(event)

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

	// Parse API Gateway response
	var apiResp struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}
	if err := json.Unmarshal(out.Payload, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal api response: %w", err)
	}
	
	if apiResp.StatusCode != 200 {
		return nil, fmt.Errorf("cart service returned status %d: %s", apiResp.StatusCode, apiResp.Body)
	}

	cart := &Cart{}
	if err := json.Unmarshal([]byte(apiResp.Body), cart); err != nil {
		return nil, fmt.Errorf("unmarshal cart: %w", err)
	}
	return cart, nil
}

func (c *CartClient) EmptyCart(ctx context.Context, userID string) error {
	// Build API Gateway V2 HTTP request event
	event := map[string]interface{}{
		"rawPath": "/cart",
		"requestContext": map[string]interface{}{
			"http": map[string]interface{}{
				"method": "DELETE",
			},
		},
		"queryStringParameters": map[string]string{
			"user_id": userID,
		},
	}
	
	payload, _ := json.Marshal(event)

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
	
	// Parse API Gateway response
	var apiResp struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}
	if err := json.Unmarshal(out.Payload, &apiResp); err != nil {
		return fmt.Errorf("unmarshal api response: %w", err)
	}
	
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("cart service returned status %d: %s", apiResp.StatusCode, apiResp.Body)
	}
	
	return nil
}

func (c *CartClient) AddItem(ctx context.Context, userID, productID string, quantity int32) error {
	// Build request body
	body := map[string]interface{}{
		"user_id":    userID,
		"product_id": productID,
		"quantity":   quantity,
	}
	bodyJSON, _ := json.Marshal(body)
	
	// Build API Gateway V2 HTTP request event
	event := map[string]interface{}{
		"rawPath": "/cart",
		"requestContext": map[string]interface{}{
			"http": map[string]interface{}{
				"method": "POST",
			},
		},
		"body": string(bodyJSON),
	}
	
	payload, _ := json.Marshal(event)

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
	
	// Parse API Gateway response
	var apiResp struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}
	if err := json.Unmarshal(out.Payload, &apiResp); err != nil {
		return fmt.Errorf("unmarshal api response: %w", err)
	}
	
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("cart service returned status %d: %s", apiResp.StatusCode, apiResp.Body)
	}
	
	return nil
}
