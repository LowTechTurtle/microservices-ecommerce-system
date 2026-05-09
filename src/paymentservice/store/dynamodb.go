package store

import (
	"context"
	"fmt"
	"time"

	"paymentservice/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoPaymentStore struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoPaymentStore(ctx context.Context, tableName string) (*DynamoPaymentStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &DynamoPaymentStore{
		client:    dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}, nil
}

func (s *DynamoPaymentStore) Create(ctx context.Context, p *models.Payment) error {
	item, err := attributevalue.MarshalMap(p)
	if err != nil {
		return fmt.Errorf("marshal payment: %w", err)
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(paymentId)"),
	})
	return err
}

func (s *DynamoPaymentStore) GetByID(ctx context.Context, paymentID string) (*models.Payment, error) {
	key, _ := attributevalue.MarshalMap(map[string]string{"paymentId": paymentID})
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("get payment: %w", err)
	}
	if out.Item == nil {
		return nil, ErrNotFound
	}
	return unmarshalPayment(out.Item)
}

func (s *DynamoPaymentStore) GetByOrderID(ctx context.Context, orderID string) (*models.Payment, error) {
	return s.queryIndex(ctx, "orderId-index", "orderId", orderID)
}

func (s *DynamoPaymentStore) GetByStripeID(ctx context.Context, stripeID string) (*models.Payment, error) {
	return s.queryIndex(ctx, "stripePaymentIntentId-index", "stripePaymentIntentId", stripeID)
}

func (s *DynamoPaymentStore) UpdateStatus(ctx context.Context, paymentID string, expectedFrom, to models.PaymentStatus, meta StatusMeta) error {
	key, _ := attributevalue.MarshalMap(map[string]string{"paymentId": paymentID})

	update := "SET #s = :to, updatedAt = :now"
	exprNames := map[string]string{"#s": "status"}
	exprValues := map[string]ddbtypes.AttributeValue{
		":from": &ddbtypes.AttributeValueMemberS{Value: string(expectedFrom)},
		":to":   &ddbtypes.AttributeValueMemberS{Value: string(to)},
		":now":  &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)},
	}

	if meta.StripeRefundID != "" {
		update += ", stripeRefundId = :rid"
		exprValues[":rid"] = &ddbtypes.AttributeValueMemberS{Value: meta.StripeRefundID}
	}
	if meta.FailureMessage != "" {
		update += ", failureMessage = :fm"
		exprValues[":fm"] = &ddbtypes.AttributeValueMemberS{Value: meta.FailureMessage}
	}

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.tableName),
		Key:                       key,
		ConditionExpression:       aws.String("#s = :from"),
		UpdateExpression:          aws.String(update),
		ExpressionAttributeNames:  exprNames,
		ExpressionAttributeValues: exprValues,
	})
	return err
}

func (s *DynamoPaymentStore) queryIndex(ctx context.Context, indexName, keyName, keyValue string) (*models.Payment, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                aws.String(s.tableName),
		IndexName:                aws.String(indexName),
		KeyConditionExpression:   aws.String("#k = :v"),
		ExpressionAttributeNames: map[string]string{"#k": keyName},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":v": &ddbtypes.AttributeValueMemberS{Value: keyValue},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", indexName, err)
	}
	if len(out.Items) == 0 {
		return nil, ErrNotFound
	}
	return unmarshalPayment(out.Items[0])
}

func unmarshalPayment(item map[string]ddbtypes.AttributeValue) (*models.Payment, error) {
	var p models.Payment
	if err := attributevalue.UnmarshalMap(item, &p); err != nil {
		return nil, fmt.Errorf("unmarshal payment: %w", err)
	}
	return &p, nil
}
