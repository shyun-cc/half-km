// Package order owns the checkout snapshot and its fulfillment mode.
package order

import (
	"errors"
	"math"
)

type Fulfillment string

const (
	MerchantDelivery Fulfillment = "merchant_delivery"
	GroupPickup      Fulfillment = "group_pickup"
)

type Line struct {
	SKU            string
	Quantity       int64
	UnitPriceCents int64
	DiscountCents  int64
}

type Snapshot struct {
	ID                 string
	StoreID            string
	ReceivingAccountID string
	Fulfillment        Fulfillment
	DeliveryFeeCents   int64
	Lines              []Line
}

func (o Snapshot) PayableCents() (int64, error) {
	if o.ID == "" || o.StoreID == "" || o.ReceivingAccountID == "" ||
		len(o.Lines) == 0 || o.DeliveryFeeCents < 0 {
		return 0, errors.New("invalid order snapshot")
	}
	switch o.Fulfillment {
	case MerchantDelivery:
	case GroupPickup:
		if o.DeliveryFeeCents != 0 {
			return 0, errors.New("group pickup cannot charge a delivery fee")
		}
	default:
		return 0, errors.New("invalid fulfillment mode")
	}

	total := o.DeliveryFeeCents
	seen := make(map[string]bool, len(o.Lines))
	for _, line := range o.Lines {
		if line.SKU == "" || seen[line.SKU] || line.Quantity <= 0 || line.UnitPriceCents < 0 ||
			line.DiscountCents < 0 || line.UnitPriceCents > math.MaxInt64/line.Quantity {
			return 0, errors.New("invalid order line")
		}
		seen[line.SKU] = true
		gross := line.Quantity * line.UnitPriceCents
		if line.DiscountCents > gross || gross-line.DiscountCents > math.MaxInt64-total {
			return 0, errors.New("invalid order total")
		}
		total += gross - line.DiscountCents
	}
	return total, nil
}
