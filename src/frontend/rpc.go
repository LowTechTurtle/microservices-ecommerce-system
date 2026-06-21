package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/frontend/genproto"
)

// Data structures for JSON communication with Lambda functions
type LambdaMoney struct {
	CurrencyCode string `json:"currencyCode"`
	Units        int64  `json:"units"`
	Nanos        int32  `json:"nanos"`
}

type LambdaProduct struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Picture     string      `json:"picture"`
	PriceUsd    LambdaMoney `json:"priceUsd"`
	Categories  []string    `json:"categories"`
}

type LambdaListProductsResponse struct {
	Products []LambdaProduct `json:"products"`
}

func mapLambdaProductToPb(lp LambdaProduct) *pb.Product {
	return &pb.Product{
		Id:          lp.ID,
		Name:        lp.Name,
		Description: lp.Description,
		Picture:     lp.Picture,
		PriceUsd: &pb.Money{
			CurrencyCode: lp.PriceUsd.CurrencyCode,
			Units:        lp.PriceUsd.Units,
			Nanos:        lp.PriceUsd.Nanos,
		},
		Categories: lp.Categories,
	}
}

// 1. PRODUCT CATALOG SERVICE
func (fe *frontendServer) getProducts(ctx context.Context) ([]*pb.Product, error) {
	url := fe.productCatalogLambdaURL + "/products"
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("lỗi gọi Lambda (%s): %v", url, err)
	}
	defer resp.Body.Close()

	var res LambdaListProductsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON: %v", err)
	}

	var out []*pb.Product
	for _, p := range res.Products {
		out = append(out, mapLambdaProductToPb(p))
	}
	return out, nil
}

func (fe *frontendServer) getProduct(ctx context.Context, id string) (*pb.Product, error) {
	url := fe.productCatalogLambdaURL + "/products/" + id
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("lỗi gọi Lambda (%s): %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("không tìm thấy sản phẩm")
	}

	var lp LambdaProduct
	if err := json.NewDecoder(resp.Body).Decode(&lp); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON: %v", err)
	}

	return mapLambdaProductToPb(lp), nil
}

// 2. CART SERVICE
type LambdaCartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type LambdaCart struct {
	UserID string           `json:"user_id"`
	Items  []LambdaCartItem `json:"items"`
}

func (fe *frontendServer) getCart(ctx context.Context, userID string) ([]*pb.CartItem, error) {
	url := fe.cartServiceLambdaURL + "/cart?user_id=" + userID
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("lỗi gọi Lambda Cart (%s): %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lỗi lấy giỏ hàng: status code %d", resp.StatusCode)
	}

	var cart LambdaCart
	if err := json.NewDecoder(resp.Body).Decode(&cart); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON giỏ hàng: %v", err)
	}

	var out []*pb.CartItem
	for _, item := range cart.Items {
		out = append(out, &pb.CartItem{
			ProductId: item.ProductID,
			Quantity:  item.Quantity,
		})
	}
	return out, nil
}

func (fe *frontendServer) emptyCart(ctx context.Context, userID string) error {
	url := fe.cartServiceLambdaURL + "/cart?user_id=" + userID
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("lỗi gọi Lambda Cart (%s): %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lỗi xóa giỏ hàng: status code %d", resp.StatusCode)
	}
	return nil
}

func (fe *frontendServer) insertCart(ctx context.Context, userID, productID string, quantity int32) error {
	url := fe.cartServiceLambdaURL + "/cart"
	payload := map[string]interface{}{
		"user_id":    userID,
		"product_id": productID,
		"quantity":   quantity,
	}
	b, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("lỗi gọi Lambda Cart (%s): %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lỗi thêm vào giỏ hàng: status code %d", resp.StatusCode)
	}
	return nil
}

// 3. SHIPPING SERVICE
type Address struct {
	StreetAddress string `json:"street_address"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	ZipCode       int32  `json:"zip_code"`
}

type ShippingQuoteRequest struct {
	Items []LambdaCartItem `json:"items"`
}

type ShippingQuoteResponse struct {
	CostUsd struct {
		CurrencyCode string `json:"currency_code"`
		Units        int64  `json:"units"`
		Nanos        int32  `json:"nanos"`
	} `json:"cost_usd"`
}

func (fe *frontendServer) getShippingQuote(ctx context.Context, items []*pb.CartItem, currency string) (*pb.Money, error) {
	url := fe.shippingLambdaURL + "/shipping/quote"
	var reqItems []LambdaCartItem
	for _, item := range items {
		reqItems = append(reqItems, LambdaCartItem{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
		})
	}
	
	payload := ShippingQuoteRequest{Items: reqItems}
	b, _ := json.Marshal(payload)
	
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("lỗi gọi Lambda Shipping Quote (%s): %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lỗi lấy phí ship: status code %d", resp.StatusCode)
	}

	var res ShippingQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("lỗi parse JSON phí ship: %v", err)
	}

	return &pb.Money{
		CurrencyCode: res.CostUsd.CurrencyCode,
		Units:        res.CostUsd.Units,
		Nanos:        res.CostUsd.Nanos,
	}, nil
}

// 4. CHECKOUT SERVICE
type ShipOrderRequest struct {
	Address Address          `json:"address"`
	Items   []LambdaCartItem `json:"items"`
}

type ShipOrderResponse struct {
	TrackingID string `json:"tracking_id"`
}

func (fe *frontendServer) checkout(ctx context.Context, req *pb.PlaceOrderRequest) (*pb.OrderResult, error) {
	// Lấy giỏ hàng của user
	cartItems, err := fe.getCart(ctx, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("không thể lấy giỏ hàng để thanh toán: %v", err)
	}

	// Gọi Lambda Shipping để giả lập vận chuyển
	url := fe.shippingLambdaURL + "/shipping/ship"
	var reqItems []LambdaCartItem
	var orderItems []*pb.OrderItem
	for _, item := range cartItems {
		reqItems = append(reqItems, LambdaCartItem{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
		})
		orderItems = append(orderItems, &pb.OrderItem{
			Item: item,
			Cost: &pb.Money{CurrencyCode: "USD", Units: 10, Nanos: 0}, // Mock cost
		})
	}

	shipPayload := ShipOrderRequest{
		Address: Address{
			StreetAddress: req.Address.StreetAddress,
			City:          req.Address.City,
			State:         req.Address.State,
			Country:       req.Address.Country,
			ZipCode:       req.Address.ZipCode,
		},
		Items: reqItems,
	}
	b, _ := json.Marshal(shipPayload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	var trackingID string = "TRACKING-ERROR"
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var res ShipOrderResponse
			json.NewDecoder(resp.Body).Decode(&res)
			trackingID = res.TrackingID
		}
	}

	// Lấy phí ship thực tế từ danh sách hàng
	shippingCost, _ := fe.getShippingQuote(ctx, cartItems, "USD")
	if shippingCost == nil {
		shippingCost = &pb.Money{CurrencyCode: "USD", Units: 8, Nanos: 990000000}
	}

	// Xóa giỏ hàng sau khi thanh toán thành công
	fe.emptyCart(ctx, req.UserId)

	return &pb.OrderResult{
		OrderId:            "MOCK-ORDER-12345",
		ShippingTrackingId: trackingID,
		ShippingCost:       shippingCost,
		ShippingAddress:    req.Address,
		Items:              orderItems,
	}, nil
}

