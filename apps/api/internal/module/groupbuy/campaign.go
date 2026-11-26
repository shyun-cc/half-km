// Package groupbuy freezes eligible paid orders in one sorting batch per campaign.
package groupbuy

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
	"github.com/shyun-cc/half-km/apps/api/internal/module/payment"
)

type Checkout struct {
	CampaignID string
	Order      order.Snapshot
	Payment    payment.Payment
}

type SortingLine struct {
	SKU      string
	Quantity int64
}

type Batch struct {
	CampaignID    string
	PickupPointID string
	ClosedAt      time.Time
	OrderIDs      []string
	SortingLines  []SortingLine
}

type Campaign struct {
	ID            string
	StoreID       string
	PickupPointID string
	Cutoff        time.Time

	batch *Batch
}

func (c *Campaign) Close(now time.Time, checkouts []Checkout) (Batch, error) {
	if c.batch != nil {
		return c.batch.snapshot(), nil
	}
	if c.ID == "" || c.StoreID == "" || c.PickupPointID == "" || c.Cutoff.IsZero() || now.Before(c.Cutoff) {
		return Batch{}, errors.New("campaign cannot be closed")
	}

	batch := Batch{CampaignID: c.ID, PickupPointID: c.PickupPointID, ClosedAt: now}
	quantities := make(map[string]int64)
	seen := make(map[string]bool, len(checkouts))
	for _, checkout := range checkouts {
		o, p := checkout.Order, checkout.Payment
		payable, err := o.PayableCents()
		if err != nil {
			return Batch{}, err
		}
		if checkout.CampaignID != c.ID || seen[o.ID] || o.StoreID != c.StoreID || o.Fulfillment != order.GroupPickup ||
			p.OrderID != o.ID || p.AccountID != o.ReceivingAccountID || p.AmountCents != payable ||
			!p.Cutoff.Equal(c.Cutoff) {
			return Batch{}, errors.New("checkout does not match campaign")
		}
		seen[o.ID] = true
		if !p.IncludedAt(c.Cutoff) {
			continue
		}
		batch.OrderIDs = append(batch.OrderIDs, o.ID)
		for _, line := range o.Lines {
			if line.Quantity > math.MaxInt64-quantities[line.SKU] {
				return Batch{}, errors.New("sorting quantity overflow")
			}
			quantities[line.SKU] += line.Quantity
		}
	}
	for sku, quantity := range quantities {
		batch.SortingLines = append(batch.SortingLines, SortingLine{SKU: sku, Quantity: quantity})
	}
	slices.Sort(batch.OrderIDs)
	slices.SortFunc(batch.SortingLines, func(a, b SortingLine) int { return strings.Compare(a.SKU, b.SKU) })
	c.batch = &batch
	return batch.snapshot(), nil
}

func (b Batch) snapshot() Batch {
	b.OrderIDs = slices.Clone(b.OrderIDs)
	b.SortingLines = slices.Clone(b.SortingLines)
	return b
}
