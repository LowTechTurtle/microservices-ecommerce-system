package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"checkoutservice/client"
	"checkoutservice/models"
	"checkoutservice/pricing"
	"checkoutservice/store"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
)

type checkoutServer struct {
	orders  store.OrderStore
	cart    *client.CartClient
	catalog *client.CatalogClient
	payment client.PaymentClient
}

func (s *checkoutServer) handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	method := req.RequestContext.HTTP.Method
	path := req.RawPath

	if method == "POST" && path == "/checkout" {
		return s.createCheckout(ctx, req)
	}
	if method == "GET" && strings.HasPrefix(path, "/checkout/") {
		orderID := strings.TrimPrefix(path, "/checkout/")
		return s.getOrder(ctx, orderID)
	}

	return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found: " + method + " " + path})
}

func (s *checkoutServer) createCheckout(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var in models.CheckoutRequest
	if err := json.Unmarshal([]byte(req.Body), &in); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if in.UserID == "" || in.Currency == "" {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "userId and currency are required"})
	}

	cart, err := s.cart.GetCart(ctx, in.UserID)
	if err != nil {
		log.WithError(err).Error("get cart failed")
		return jsonResponse(http.StatusBadGateway, map[string]string{"error": "cart unavailable"})
	}
	if len(cart.Items) == 0 {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "cart is empty"})
	}

	// Re-price every line against the authoritative catalog.
	lines := make([]pricing.LineItem, 0, len(cart.Items))
	for _, it := range cart.Items {
		p, err := s.catalog.GetProduct(ctx, it.ProductID)
		if err != nil {
			log.WithError(err).WithField("productId", it.ProductID).Error("catalog lookup failed")
			return jsonResponse(http.StatusBadGateway, map[string]string{"error": "catalog unavailable"})
		}
		if p.PriceUsd.CurrencyCode != in.Currency {
			// TODO: integrate a currency conversion service.
			return jsonResponse(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("currency mismatch for %s: %s vs %s", it.ProductID, p.PriceUsd.CurrencyCode, in.Currency),
			})
		}
		lines = append(lines, pricing.LineItem{
			ProductID: p.ID,
			Name:      p.Name,
			Quantity:  it.Quantity,
			UnitPrice: p.PriceUsd,
		})
	}

	quote := pricing.Calculate(in.Currency, lines)
	now := time.Now().UTC()
	order := &models.Order{
		OrderID:         uuid.NewString(),
		UserID:          in.UserID,
		Status:          models.StatusPending,
		Items:           quote.Items,
		Subtotal:        quote.Subtotal,
		Tax:             quote.Tax,
		Shipping:        quote.Shipping,
		Total:           quote.Total,
		Currency:        in.Currency,
		ShippingAddress: in.ShippingAddress,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.orders.CreateOrder(ctx, order); err != nil {
		log.WithError(err).Error("create order failed")
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": "could not create order"})
	}

	intent, err := s.payment.CreateIntent(ctx, order.OrderID, order.UserID, order.Total, order.Currency)
	if err != nil {
		log.WithError(err).Error("create payment intent failed")
		// Order is left in `pending`; a sweeper or user-initiated cancel can clean it up.
		return jsonResponse(http.StatusBadGateway, map[string]string{"error": "payment unavailable"})
	}

	return jsonResponse(http.StatusOK, models.CheckoutResponse{
		OrderID:      order.OrderID,
		ClientSecret: intent.ClientSecret,
		Total:        order.Total,
	})
}

func (s *checkoutServer) getOrder(ctx context.Context, orderID string) (events.APIGatewayV2HTTPResponse, error) {
	order, err := s.orders.GetOrder(ctx, orderID)
	if err == store.ErrOrderNotFound {
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "order not found"})
	}
	if err != nil {
		log.WithError(err).Error("get order failed")
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": "could not load order"})
	}
	return jsonResponse(http.StatusOK, order)
}

func jsonResponse(status int, body interface{}) (events.APIGatewayV2HTTPResponse, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusInternalServerError, Body: `{"error":"marshal failed"}`}, nil
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}, nil
}
