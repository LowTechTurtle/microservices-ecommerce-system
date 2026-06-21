// --- START OF FILE main.go ---

package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"cartservice/store"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var cartStore store.CartStore

func init() {
	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "Carts"
	}

	var err error
	cartStore, err = store.NewDynamoCartStore(context.Background(), tableName)
	if err != nil {
		log.Fatalf("Failed to initialize DynamoDB: %v", err)
	}
}

// Hàm Helper để trả về JSON
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

	if path == "/cart" {
		// 1. LẤY GIỎ HÀNG (GET /cart?user_id=...)
		if method == "GET" {
			userID := req.QueryStringParameters["user_id"]
			cart, err := cartStore.GetCart(ctx, userID)
			if err != nil {
				return respondJSON(500, map[string]string{"error": err.Error()})
			}
			return respondJSON(200, cart)
		}

		// 2. THÊM VÀO GIỎ HÀNG (POST /cart)
		if method == "POST" {
			var body struct {
				UserID    string `json:"user_id"`
				ProductID string `json:"product_id"`
				Quantity  int32  `json:"quantity"`
			}
			if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
				return respondJSON(400, map[string]string{"error": "Invalid JSON body"})
			}

			err := cartStore.AddItem(ctx, body.UserID, body.ProductID, body.Quantity)
			if err != nil {
				return respondJSON(500, map[string]string{"error": err.Error()})
			}
			return respondJSON(200, map[string]string{"status": "success"})
		}

		// 3. XÓA GIỎ HÀNG (DELETE /cart?user_id=...)
		if method == "DELETE" {
			userID := req.QueryStringParameters["user_id"]
			err := cartStore.EmptyCart(ctx, userID)
			if err != nil {
				return respondJSON(500, map[string]string{"error": err.Error()})
			}
			return respondJSON(200, map[string]string{"status": "success"})
		}
	}

	return events.APIGatewayV2HTTPResponse{StatusCode: 404, Body: "Not Found"}, nil
}

func main() {
	lambda.Start(HandleRequest)
}
