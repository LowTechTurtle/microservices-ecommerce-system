package models

type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type Cart struct {
	UserID string      `json:"user_id"`
	Items[]*CartItem `json:"items"`
}

type AddItemRequest struct {
	UserID string    `json:"user_id"`
	Item   *CartItem `json:"item"`
}

type GetCartRequest struct {
	UserID string `json:"user_id"`
}

type EmptyCartRequest struct {
	UserID string `json:"user_id"`
}
