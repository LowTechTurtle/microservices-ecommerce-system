package store

import (
	"context"
	"fmt"
	"log"

	"cartservice/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type DynamoCartStore struct {
	client    *dynamodb.Client
	tableName string
}

type CartRecord struct {
	UserID string           `dynamodbav:"user_id"`
	Items  map[string]int32 `dynamodbav:"items"`
}

func NewDynamoCartStore(ctx context.Context, tableName string) (*DynamoCartStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS config: %w", err)
	}

	return &DynamoCartStore{
		client:    dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}, nil
}

func (s *DynamoCartStore) AddItem(ctx context.Context, userID, productID string, quantity int32) error {
	log.Printf("AddItem called for user %s, product %s", userID, productID)

	record, err := s.getCartRecord(ctx, userID)
	if err != nil {
		return err
	}

	record.Items[productID] += quantity

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	return err
}

// Fixed the signature to use *models.Cart instead of *pb.Cart
func (s *DynamoCartStore) GetCart(ctx context.Context, userID string) (*models.Cart, error) {
	log.Printf("GetCart called for user %s", userID)

	record, err := s.getCartRecord(ctx, userID)
	if err != nil {
		return nil, err
	}

	cart := &models.Cart{UserID: userID}
	for productID, qty := range record.Items {
		cart.Items = append(cart.Items, &models.CartItem{
			ProductID: productID,
			Quantity:  qty,
		})
	}
	return cart, nil
}

func (s *DynamoCartStore) EmptyCart(ctx context.Context, userID string) error {
	log.Printf("EmptyCart called for user %s", userID)

	key, _ := attributevalue.MarshalMap(map[string]string{"user_id": userID})
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	return err
}

func (s *DynamoCartStore) Ping(ctx context.Context) error {
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	})
	return err
}

func (s *DynamoCartStore) getCartRecord(ctx context.Context, userID string) (*CartRecord, error) {
	key, _ := attributevalue.MarshalMap(map[string]string{"user_id": userID})

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	record := &CartRecord{
		UserID: userID,
		Items:  make(map[string]int32),
	}

	if result.Item != nil {
		if err := attributevalue.UnmarshalMap(result.Item, record); err != nil {
			return nil, fmt.Errorf("failed to unmarshal item: %w", err)
		}
		if record.Items == nil {
			record.Items = make(map[string]int32)
		}
	}
	return record, nil
}
