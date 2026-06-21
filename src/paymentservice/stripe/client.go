package stripe

import (
	"fmt"

	"paymentservice/models"

	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/paymentintent"
	"github.com/stripe/stripe-go/v76/refund"
)

type Client struct{}

func NewClient(secretKey string) *Client {
	stripe.Key = secretKey
	return &Client{}
}

// IntentResult holds everything returned by Stripe after creating a PaymentIntent.
type IntentResult struct {
	StripePaymentIntentID string
	ClientSecret          string
	// QRCodeData is the raw string to encode as a QR image.
	// Populated for wechat_pay and promptpay intents.
	QRCodeData  string
	// RedirectURL is populated for alipay — user opens this URL to complete payment.
	RedirectURL string
}

// CreateIntent creates a Stripe PaymentIntent for the given method type.
// For card payments the frontend uses ClientSecret with Stripe Elements.
// For QR-based methods (wechat_pay, promptpay) Stripe returns QR data in
// next_action after the intent reaches requires_action state.
func (c *Client) CreateIntent(req *models.CreateIntentRequest) (*IntentResult, error) {
	method := req.PaymentMethod
	if method == "" {
		method = models.MethodCard
	}

	amount := models.MoneyToStripeAmount(req.Amount)
	currency := stripe.String(req.Currency)

	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(amount),
		Currency:           currency,
		PaymentMethodTypes: stripe.StringSlice([]string{string(method)}),
	}

	// wechat_pay requires a mandate_data and payment_method_options block.
	if method == models.MethodWechatPay {
		params.PaymentMethodOptions = &stripe.PaymentIntentPaymentMethodOptionsParams{
			WechatPay: &stripe.PaymentIntentPaymentMethodOptionsWechatPayParams{
				// "web" shows the QR code inline; "native" deep-links into the app.
				Client: stripe.String("web"),
			},
		}
	}

	// Idempotency key prevents duplicate charges on Lambda retries.
	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe create intent: %w", err)
	}

	result := &IntentResult{
		StripePaymentIntentID: pi.ID,
		ClientSecret:          pi.ClientSecret,
	}

	// Extract QR / redirect data if Stripe already populated next_action.
	// For QR-based methods Stripe sets this when the intent reaches
	// requires_action — it may not be present immediately on creation.
	if pi.NextAction != nil {
		switch {
		case pi.NextAction.WechatPayDisplayQrCode != nil:
			result.QRCodeData = pi.NextAction.WechatPayDisplayQrCode.Data
		case pi.NextAction.PromptPayDisplayQrCode != nil:
			result.QRCodeData = pi.NextAction.PromptPayDisplayQrCode.Data
		case pi.NextAction.AlipayHandleRedirect != nil:
			result.RedirectURL = pi.NextAction.AlipayHandleRedirect.URL
		}
	}

	return result, nil
}

// RefundResult holds Stripe's response after creating a refund.
type RefundResult struct {
	StripeRefundID string
}

// Refund issues a full or partial refund against a Stripe PaymentIntent.
// Pass amountMinorUnits = 0 for a full refund.
func (c *Client) Refund(stripePaymentIntentID string, amountMinorUnits int64) (*RefundResult, error) {
	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(stripePaymentIntentID),
	}
	if amountMinorUnits > 0 {
		params.Amount = stripe.Int64(amountMinorUnits)
	}

	r, err := refund.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
	}
	return &RefundResult{StripeRefundID: r.ID}, nil
}
