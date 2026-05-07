package pricing

import (
	"checkoutservice/models"
)

const nanosPerUnit = 1_000_000_000

// Quote summarises the priced order before persistence.
type Quote struct {
	Items    []models.OrderItem
	Subtotal models.Money
	Tax      models.Money
	Shipping models.Money
	Total    models.Money
}

// LineItem is the input to the calculator: a cart entry resolved against
// an authoritative catalog price.
type LineItem struct {
	ProductID string
	Name      string
	Quantity  int32
	UnitPrice models.Money
}

// TaxRate is a fixed percentage applied to the subtotal.
// Replace with a tax service / per-jurisdiction lookup later.
const TaxRate = 0.08

// FlatShippingUnits is a flat per-order shipping fee (in the order's currency).
const FlatShippingUnits int64 = 5

// Calculate prices an order. All amounts must share a single currency.
func Calculate(currency string, items []LineItem) Quote {
	var subtotalNanos int64
	priced := make([]models.OrderItem, 0, len(items))

	for _, it := range items {
		lineNanos := moneyToNanos(it.UnitPrice) * int64(it.Quantity)
		lineTotal := nanosToMoney(currency, lineNanos)
		priced = append(priced, models.OrderItem{
			ProductID: it.ProductID,
			Name:      it.Name,
			Quantity:  it.Quantity,
			UnitPrice: it.UnitPrice,
			LineTotal: lineTotal,
		})
		subtotalNanos += lineNanos
	}

	subtotal := nanosToMoney(currency, subtotalNanos)
	tax := nanosToMoney(currency, int64(float64(subtotalNanos)*TaxRate))
	shipping := models.Money{CurrencyCode: currency, Units: FlatShippingUnits, Nanos: 0}

	totalNanos := subtotalNanos + moneyToNanos(tax) + moneyToNanos(shipping)
	total := nanosToMoney(currency, totalNanos)

	return Quote{
		Items:    priced,
		Subtotal: subtotal,
		Tax:      tax,
		Shipping: shipping,
		Total:    total,
	}
}

func moneyToNanos(m models.Money) int64 {
	return m.Units*nanosPerUnit + int64(m.Nanos)
}

func nanosToMoney(currency string, nanos int64) models.Money {
	return models.Money{
		CurrencyCode: currency,
		Units:        nanos / nanosPerUnit,
		Nanos:        int32(nanos % nanosPerUnit),
	}
}
