package groupbuy

import (
	"reflect"
	"testing"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
	"github.com/shyun-cc/half-km/apps/api/internal/module/payment"
)

func TestCloseFreezesPaidQuantityAndReusesBatch(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	c := Campaign{ID: "campaign", StoreID: "store", PickupPointID: "point", Cutoff: cutoff}
	if _, err := c.Close(cutoff.Add(-time.Second), nil); err == nil {
		t.Fatal("closed before the cutoff")
	}

	newCheckout := func(id string, quantity int64, paid bool) Checkout {
		t.Helper()
		o := order.Snapshot{
			ID: id, StoreID: c.StoreID, ReceivingAccountID: "account", Fulfillment: order.GroupPickup,
			Lines: []order.Line{{SKU: "apple", Quantity: quantity, UnitPriceCents: 10}},
		}
		p := payment.Payment{OrderID: id, AccountID: o.ReceivingAccountID, AmountCents: quantity * 10, Cutoff: cutoff, State: payment.Pending}
		if paid {
			before := cutoff.Add(-time.Second)
			receipt := payment.Receipt{OrderID: id, AccountID: p.AccountID, TransactionID: "trade-" + id, AmountCents: p.AmountCents, PaidAt: before}
			if _, err := p.Record(receipt, before); err != nil {
				t.Fatal(err)
			}
		}
		return Checkout{CampaignID: c.ID, Order: o, Payment: p}
	}

	foreign := newCheckout("wrong-campaign", 1, true)
	foreign.CampaignID = "different"
	if _, err := c.Close(cutoff, []Checkout{foreign}); err == nil {
		t.Fatal("included an order from another campaign")
	}
	checkouts := []Checkout{newCheckout("b", 1, true), newCheckout("unpaid", 10, false), newCheckout("a", 2, true)}
	batch, err := c.Close(cutoff, checkouts)
	want := Batch{
		CampaignID: c.ID, PickupPointID: c.PickupPointID, ClosedAt: cutoff,
		OrderIDs: []string{"a", "b"}, SortingLines: []SortingLine{{SKU: "apple", Quantity: 3}},
	}
	if err != nil || !reflect.DeepEqual(batch, want) {
		t.Fatalf("first close: %+v, %v", batch, err)
	}

	batch.OrderIDs[0] = "changed"
	batch.SortingLines[0].Quantity = 50
	replay, err := c.Close(cutoff.Add(time.Hour), []Checkout{newCheckout("changed", 20, true)})
	if err != nil || !reflect.DeepEqual(replay, want) {
		t.Fatalf("repeated close changed the batch: %+v, %v", replay, err)
	}
}

func TestCloseWithoutPaidOrders(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	c := Campaign{ID: "campaign", StoreID: "store", PickupPointID: "point", Cutoff: cutoff}
	batch, err := c.Close(cutoff, nil)
	if err != nil || batch.CampaignID != c.ID || len(batch.OrderIDs) != 0 || len(batch.SortingLines) != 0 {
		t.Fatalf("closing with zero participants: %+v, %v", batch, err)
	}
}
