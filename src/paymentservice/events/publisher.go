package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

const source = "paymentservice"

// PaymentEvent is the EventBridge detail payload.
// checkoutservice/paymentconsumer/main.go reads this exact shape.
type PaymentEvent struct {
	OrderID   string `json:"orderId"`
	PaymentID string `json:"paymentId"`
}

type Publisher struct {
	eb      *eventbridge.Client
	busName string
}

func NewPublisher(eb *eventbridge.Client, busName string) *Publisher {
	return &Publisher{eb: eb, busName: busName}
}

func (p *Publisher) PaymentSucceeded(ctx context.Context, orderID, paymentID string) error {
	return p.publish(ctx, "PaymentSucceeded", PaymentEvent{OrderID: orderID, PaymentID: paymentID})
}

func (p *Publisher) PaymentFailed(ctx context.Context, orderID, paymentID string) error {
	return p.publish(ctx, "PaymentFailed", PaymentEvent{OrderID: orderID, PaymentID: paymentID})
}

func (p *Publisher) publish(ctx context.Context, detailType string, detail interface{}) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal event detail: %w", err)
	}

	_, err = p.eb.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{
			{
				EventBusName: aws.String(p.busName),
				Source:       aws.String(source),
				DetailType:   aws.String(detailType),
				Detail:       aws.String(string(b)),
				Time:         aws.Time(time.Now().UTC()),
			},
		},
	})
	return err
}
