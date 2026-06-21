package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"checkoutservice/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var ErrOrderNotFound = errors.New("order not found")

type DynamoOrderStore struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoOrderStore(ctx context.Context, tableName string) (*DynamoOrderStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS config: %w", err)
	}
	return &DynamoOrderStore{
		client:    dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}, nil
}

func (s *DynamoOrderStore) CreateOrder(ctx context.Context, order *models.Order) error {
	item, err := attributevalue.MarshalMap(order)
	if err != nil {
		return fmt.Errorf("marshal order: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(orderId)"),
	})
	return err
}

func (s *DynamoOrderStore) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	key, _ := attributevalue.MarshalMap(map[string]string{"orderId": orderID})

	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	if out.Item == nil {
		return nil, ErrOrderNotFound
	}

	var order models.Order
	if err := attributevalue.UnmarshalMap(out.Item, &order); err != nil {
		return nil, fmt.Errorf("unmarshal order: %w", err)
	}
	return &order, nil
}

func (s *DynamoOrderStore) UpdateStatus(ctx context.Context, orderID string, expectedFrom, to models.OrderStatus, paymentID string) error {
	key, _ := attributevalue.MarshalMap(map[string]string{"orderId": orderID})

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.tableName),
		Key:                 key,
		ConditionExpression: aws.String("#s = :from"),
		UpdateExpression:    aws.String("SET #s = :to, paymentId = :pid, updatedAt = :now"),
		ExpressionAttributeNames: map[string]string{
			"#s": "status",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":from": &ddbtypes.AttributeValueMemberS{Value: string(expectedFrom)},
			":to":   &ddbtypes.AttributeValueMemberS{Value: string(to)},
			":pid":  &ddbtypes.AttributeValueMemberS{Value: paymentID},
			":now":  &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)},
		},
	})
	return err
}
