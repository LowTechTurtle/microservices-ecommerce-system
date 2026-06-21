package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type Address struct {
	StreetAddress string `json:"street_address"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	ZipCode       int32  `json:"zip_code"`
}

type GetQuoteRequest struct {
	Items []CartItem `json:"items"`
}

type GetQuoteResponse struct {
	CostUsd struct {
		CurrencyCode string `json:"currency_code"`
		Units        int64  `json:"units"`
		Nanos        int32  `json:"nanos"`
	} `json:"cost_usd"`
}

type ShipOrderRequest struct {
	Address Address    `json:"address"`
	Items   []CartItem `json:"items"`
}

type ShipOrderResponse struct {
	TrackingID string `json:"tracking_id"`
}

func respondJSON(statusCode int, body interface{}) (events.APIGatewayV2HTTPResponse, error) {
	b, _ := json.Marshal(body)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}, nil
}

func HandleRequest(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	method := req.RequestContext.HTTP.Method
	path := req.RawPath

	if path == "/shipping/quote" && method == "POST" {
		var body GetQuoteRequest
		if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
			return respondJSON(400, map[string]string{"error": "Invalid JSON body"})
		}

		count := 0
		for _, item := range body.Items {
			count += int(item.Quantity)
		}
		quote := CreateQuoteFromCount(count)

		var resp GetQuoteResponse
		resp.CostUsd.CurrencyCode = "USD"
		resp.CostUsd.Units = int64(quote.Dollars)
		resp.CostUsd.Nanos = int32(quote.Cents * 10000000)

		return respondJSON(200, resp)
	}

	if path == "/shipping/ship" && method == "POST" {
		var body ShipOrderRequest
		if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
			return respondJSON(400, map[string]string{"error": "Invalid JSON body"})
		}

		baseAddress := fmt.Sprintf("%s, %s, %s", body.Address.StreetAddress, body.Address.City, body.Address.State)
		id := CreateTrackingId(baseAddress)

		resp := ShipOrderResponse{
			TrackingID: id,
		}
		return respondJSON(200, resp)
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: 404, Body: "Not Found"}, nil
}

func main() {
	lambda.Start(HandleRequest)
}
