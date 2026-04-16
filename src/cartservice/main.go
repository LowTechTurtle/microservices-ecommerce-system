package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"cartservice/models"
	"cartservice/store"

	"github.com/aws/aws-lambda-go/lambda"
)

type CartLambdaEvent struct {
	Action       string                   `json:"action"` // "AddItem", "GetCart", or "EmptyCart"
	AddItemReq   *models.AddItemRequest   `json:"add_item_req,omitempty"`
	GetCartReq   *models.GetCartRequest   `json:"get_cart_req,omitempty"`
	EmptyCartReq *models.EmptyCartRequest `json:"empty_cart_req,omitempty"`
}

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

func HandleRequest(ctx context.Context, event CartLambdaEvent) (interface{}, error) {
	switch event.Action {
	case "AddItem":
		// Prevent crash if the request is malformed
		if event.AddItemReq == nil || event.AddItemReq.Item == nil {
			return nil, fmt.Errorf("invalid or missing add_item_req")
		}
		err := cartStore.AddItem(ctx, event.AddItemReq.UserID, event.AddItemReq.Item.ProductID, event.AddItemReq.Item.Quantity)
		return map[string]string{"status": "success"}, err

	case "GetCart":
		// Prevent crash if the request is malformed
		if event.GetCartReq == nil {
			return nil, fmt.Errorf("invalid or missing get_cart_req")
		}
		return cartStore.GetCart(ctx, event.GetCartReq.UserID)

	case "EmptyCart":
		// Prevent crash if the request is malformed
		if event.EmptyCartReq == nil {
			return nil, fmt.Errorf("invalid or missing empty_cart_req")
		}
		err := cartStore.EmptyCart(ctx, event.EmptyCartReq.UserID)
		return map[string]string{"status": "success"}, err

	default:
		return nil, fmt.Errorf("unknown action: %s", event.Action)
	}
}

func main() {
	lambda.Start(HandleRequest)
}
