package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// --- Native Go Models (Replacing Protobuf) ---
type Money struct {
	CurrencyCode string `json:"currencyCode"`
	Units        int64  `json:"units"`
	Nanos        int32  `json:"nanos"`
}

type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Picture     string   `json:"picture"`
	PriceUsd    Money    `json:"priceUsd"`
	Categories  []string `json:"categories"`
}

// Wrapper structs to ensure the JSON responses match the exact format the frontend expects
type ListProductsResponse struct {
	Products []Product `json:"products"`
}

type SearchProductsResponse struct {
	Results []Product `json:"results"`
}

type productCatalog struct {
	products []Product
}

// LambdaHandler routes incoming AWS Lambda Function URL HTTP requests (Payload V2)
func (p *productCatalog) LambdaHandler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	path := req.RawPath
	method := req.RequestContext.HTTP.Method

	if method == "GET" && path == "/products" {
		return p.ListProducts(ctx)
	}

	if method == "GET" && strings.HasPrefix(path, "/products/search") {
		query := req.QueryStringParameters["query"]
		return p.SearchProducts(ctx, query)
	}

	if method == "GET" && strings.HasPrefix(path, "/products/") && path != "/products/reload" {
		id := strings.TrimPrefix(path, "/products/")
		return p.GetProduct(ctx, id)
	}

	if method == "POST" && path == "/products/reload" {
		err := loadCatalog(&p.products)
		if err != nil {
			return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError, Body: "Failed to reload catalog"}, nil
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusOK, Body: "Catalog reloaded successfully"}, nil
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusNotFound, Body: "Endpoint Not Found: " + method + " " + path}, nil
}

func jsonResponse(statusCode int, body interface{}) (events.APIGatewayV2HTTPResponse, error) {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError, Body: "Failed to marshal JSON"}, nil
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(jsonBytes),
	}, nil
}

func (p *productCatalog) ListProducts(ctx context.Context) (events.APIGatewayV2HTTPResponse, error) {
	catalogMutex.Lock()
	resp := ListProductsResponse{Products: p.products}
	catalogMutex.Unlock()

	return jsonResponse(http.StatusOK, resp)
}

func (p *productCatalog) GetProduct(ctx context.Context, id string) (events.APIGatewayV2HTTPResponse, error) {
	catalogMutex.Lock()
	defer catalogMutex.Unlock()

	for _, product := range p.products {
		if product.ID == id {
			return jsonResponse(http.StatusOK, product)
		}
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusNotFound, Body: "no product with ID " + id}, nil
}

func (p *productCatalog) SearchProducts(ctx context.Context, query string) (events.APIGatewayV2HTTPResponse, error) {
	var results []Product
	queryLower := strings.ToLower(query)

	catalogMutex.Lock()
	for _, product := range p.products {
		if strings.Contains(strings.ToLower(product.Name), queryLower) ||
			strings.Contains(strings.ToLower(product.Description), queryLower) {
			results = append(results, product)
		}
	}
	catalogMutex.Unlock()

	return jsonResponse(http.StatusOK, SearchProductsResponse{Results: results})
}
