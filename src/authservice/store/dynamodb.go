package store

import (
    "authservice/models"
    "context"
    "time"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
    "github.com/google/uuid"
)

type UserStore interface {
    GetUserByGoogleID(ctx context.Context, googleID string) (*models.User, error)
    GetUserByID(ctx context.Context, userID string) (*models.User, error)
    CreateUser(ctx context.Context, googleID, email, name, picture string) (*models.User, error)
    UpdateLastLogin(ctx context.Context, userID string) error
}

type DynamoUserStore struct {
    client    *dynamodb.Client
    tableName string
}

func NewDynamoUserStore(ctx context.Context, tableName string) (*DynamoUserStore, error) {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return nil, err
    }
    return &DynamoUserStore{
        client:    dynamodb.NewFromConfig(cfg),
        tableName: tableName,
    }, nil
}

func (s *DynamoUserStore) GetUserByGoogleID(ctx context.Context, googleID string) (*models.User, error) {
    result, err := s.client.Query(ctx, &dynamodb.QueryInput{
        TableName:              aws.String(s.tableName),
        IndexName:              aws.String("google_id-index"),
        KeyConditionExpression: aws.String("google_id = :gid"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":gid": &types.AttributeValueMemberS{Value: googleID},
        },
    })
    if err != nil {
        return nil, err
    }
    if len(result.Items) == 0 {
        return nil, nil
    }

    var user models.User
    if err := attributevalue.UnmarshalMap(result.Items[0], &user); err != nil {
        return nil, err
    }
    return &user, nil
}

func (s *DynamoUserStore) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
    result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(s.tableName),
        Key: map[string]types.AttributeValue{
            "user_id": &types.AttributeValueMemberS{Value: userID},
        },
    })
    if err != nil {
        return nil, err
    }
    if result.Item == nil {
        return nil, nil
    }

    var user models.User
    if err := attributevalue.UnmarshalMap(result.Item, &user); err != nil {
        return nil, err
    }
    return &user, nil
}

func (s *DynamoUserStore) CreateUser(ctx context.Context, googleID, email, name, picture string) (*models.User, error) {
    now := time.Now().UTC()
    user := &models.User{
        UserID:      uuid.NewString(),
        GoogleID:    googleID,
        Email:       email,
        Name:        name,
        Picture:     picture,
        CreatedAt:   now,
        LastLoginAt: now,
    }

    item, err := attributevalue.MarshalMap(user)
    if err != nil {
        return nil, err
    }

    _, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(s.tableName),
        Item:      item,
    })
    if err != nil {
        return nil, err
    }
    return user, nil
}

func (s *DynamoUserStore) UpdateLastLogin(ctx context.Context, userID string) error {
    now := time.Now().UTC()
    _, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
        TableName: aws.String(s.tableName),
        Key: map[string]types.AttributeValue{
            "user_id": &types.AttributeValueMemberS{Value: userID},
        },
        UpdateExpression: aws.String("SET last_login_at = :now"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":now": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
        },
    })
    return err
}
