// --- START OF FILE seeder/main.go ---
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Copy lại struct từ code của bạn
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

type ListProductsResponse struct {
	Products []Product `json:"products"`
}

type DynamoProduct struct {
	ID                   string   `dynamodbav:"id"`
	Name                 string   `dynamodbav:"name"`
	Description          string   `dynamodbav:"description"`
	Picture              string   `dynamodbav:"picture"`
	PriceUsdCurrencyCode string   `dynamodbav:"price_usd_currency_code"`
	PriceUsdUnits        int64    `dynamodbav:"price_usd_units"`
	PriceUsdNanos        int32    `dynamodbav:"price_usd_nanos"`
	Categories           []string `dynamodbav:"categories"`
}

func main() {
	// 1. Đọc file JSON
	fileData, err := os.ReadFile("../products.json") // Lùi lại 1 thư mục để đọc file
	if err != nil {
		log.Fatalf("Không thể đọc file products.json: %v", err)
	}

	var wrapper ListProductsResponse
	if err := json.Unmarshal(fileData, &wrapper); err != nil {
		log.Fatalf("Lỗi parse JSON: %v", err)
	}

	// 2. Kết nối AWS
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"))
	if err != nil {
		log.Fatalf("Lỗi kết nối AWS: %v", err)
	}
	svc := dynamodb.NewFromConfig(cfg)
	tableName := "products"

	// 3. Lặp qua từng product và đẩy lên DynamoDB
	for _, p := range wrapper.Products {
		// Chuyển đổi sang format của DynamoDB
		dp := DynamoProduct{
			ID:                   p.ID,
			Name:                 p.Name,
			Description:          p.Description,
			Picture:              p.Picture,
			PriceUsdCurrencyCode: p.PriceUsd.CurrencyCode,
			PriceUsdUnits:        p.PriceUsd.Units,
			PriceUsdNanos:        p.PriceUsd.Nanos,
			Categories:           p.Categories,
		}

		item, err := attributevalue.MarshalMap(dp)
		if err != nil {
			log.Printf("Lỗi marshal product %s: %v", p.ID, err)
			continue
		}
		_, err = svc.PutItem(context.Background(), &dynamodb.PutItemInput{
			TableName: &tableName,
			Item:      item,
		})

		if err != nil {
			log.Printf("Lỗi khi insert product %s: %v", p.ID, err)
		} else {
			fmt.Printf("Đã insert thành công: %s\n", p.Name)
		}
	}
	fmt.Println("Hoàn tất bơm dữ liệu lên DynamoDB!")
}
