package store
import (
	"context"
	"cartservice/models" // Use our new structs
)

type CartStore interface {
	AddItem(ctx context.Context, userID, productID string, quantity int32) error
	EmptyCart(ctx context.Context, userID string) error
	GetCart(ctx context.Context, userID string) (*models.Cart, error)
	Ping(ctx context.Context) error
}
