package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoProduct maps strictly to your DynamoDB table schema
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

func loadCatalog(products *[]Product) error {
	catalogMutex.Lock()
	defer catalogMutex.Unlock()

	if os.Getenv("DYNAMODB_TABLE_NAME") != "" {
		return loadCatalogFromDynamoDB(products)
	}

	return loadCatalogFromLocalFile(products)
}

func loadCatalogFromLocalFile(products *[]Product) error {
	log.Info("loading catalog from local products.json file...")

	catalogJSON, err := os.ReadFile("products.json")
	if err != nil {
		log.Warnf("failed to open product catalog json file: %v", err)
		return err
	}

	// The local products.json has a top-level "products" key
	var wrapper ListProductsResponse
	if err := json.Unmarshal(catalogJSON, &wrapper); err != nil {
		log.Warnf("failed to parse the catalog JSON: %v", err)
		return err
	}

	*products = wrapper.Products
	log.Info("successfully parsed product catalog json")
	return nil
}

func loadCatalogFromDynamoDB(products *[]Product) error {
	log.Info("loading catalog from AWS DynamoDB...")

	tableName := os.Getenv("DYNAMODB_TABLE_NAME")

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	svc := dynamodb.NewFromConfig(cfg)

	out, err := svc.Scan(context.Background(), &dynamodb.ScanInput{
		TableName: &tableName,
	})
	if err != nil {
		return fmt.Errorf("failed to scan DynamoDB table: %w", err)
	}

	var ddbProducts []DynamoProduct
	err = attributevalue.UnmarshalListOfMaps(out.Items, &ddbProducts)
	if err != nil {
		return fmt.Errorf("failed to unmarshal DynamoDB items: %w", err)
	}

	// Map the Dynamo structs directly to our Native Go structs
	*products = (*products)[:0]
	for _, dp := range ddbProducts {
		p := Product{
			ID:          dp.ID,
			Name:        dp.Name,
			Description: dp.Description,
			Picture:     dp.Picture,
			Categories:  dp.Categories,
			PriceUsd: Money{
				CurrencyCode: dp.PriceUsdCurrencyCode,
				Units:        dp.PriceUsdUnits,
				Nanos:        dp.PriceUsdNanos,
			},
		}
		*products = append(*products, p)
	}

	log.Infof("successfully parsed %d products from AWS DynamoDB", len(*products))
	return nil
}
