package client

import (
	"context"
	"encoding/json"
	"fmt"

	"checkoutservice/models"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
)

// productResponse mirrors src/productcatalogservice/server.go Product.
type productResponse struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Picture     string       `json:"picture"`
	PriceUsd    models.Money `json:"priceUsd"`
	Categories  []string     `json:"categories"`
}

type Product struct {
	ID       string
	Name     string
	PriceUsd models.Money
}

type CatalogClient struct {
	lambda       *awslambda.Client
	functionName string
}

func NewCatalogClient(lambda *awslambda.Client, functionName string) *CatalogClient {
	return &CatalogClient{lambda: lambda, functionName: functionName}
}

// GetProduct invokes productcatalogservice with a synthetic APIGatewayV2
// request, since that service routes by HTTP method + path.
func (c *CatalogClient) GetProduct(ctx context.Context, productID string) (*Product, error) {
	req := events.APIGatewayV2HTTPRequest{
		RawPath: "/products/" + productID,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: "GET",
				Path:   "/products/" + productID,
			},
		},
	}
	payload, _ := json.Marshal(req)

	out, err := c.lambda.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName: aws.String(c.functionName),
		Payload:      payload,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke catalog: %w", err)
	}
	if out.FunctionError != nil {
		return nil, fmt.Errorf("catalog error: %s", string(out.Payload))
	}

	var resp events.APIGatewayV2HTTPResponse
	if err := json.Unmarshal(out.Payload, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal catalog response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("catalog status %d: %s", resp.StatusCode, resp.Body)
	}

	var p productResponse
	if err := json.Unmarshal([]byte(resp.Body), &p); err != nil {
		return nil, fmt.Errorf("unmarshal product: %w", err)
	}
	return &Product{ID: p.ID, Name: p.Name, PriceUsd: p.PriceUsd}, nil
}
