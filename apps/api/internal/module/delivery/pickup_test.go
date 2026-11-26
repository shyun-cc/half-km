package delivery

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

func TestRedeemIdempotently(t *testing.T) {
	now := time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
	p := Pickup{OrderID: "order", BatchID: "batch", PickupPointID: "point", LeaderID: "leader",
		ArrivedAt: now, CodeDigest: sha256.Sum256([]byte("pickup-code"))}
	command := Redemption{OrderID: p.OrderID, PickupPointID: p.PickupPointID, LeaderID: p.LeaderID,
		Code: "pickup-code", EventID: "event"}

	if changed, err := p.Redeem(command, now.Add(-time.Second)); changed || !errors.Is(err, ErrPickupDenied) {
		t.Fatalf("redeemed before arrival: %v, %v", changed, err)
	}
	for _, alter := range []func(*Redemption){
		func(r *Redemption) { r.LeaderID = "other-leader" },
		func(r *Redemption) { r.PickupPointID = "other-point" },
		func(r *Redemption) { r.Code = "wrong-code" },
	} {
		bad := command
		alter(&bad)
		if changed, err := p.Redeem(bad, now); changed || !errors.Is(err, ErrPickupDenied) {
			t.Fatalf("invalid redemption: %v, %v", changed, err)
		}
	}
	if changed, err := p.Redeem(command, now); !changed || err != nil {
		t.Fatalf("first redemption: %v, %v", changed, err)
	}
	replayAt := now.Add(time.Minute)
	if changed, err := p.Redeem(command, replayAt); changed || err != nil || !p.RedeemedAt().Equal(now) {
		t.Fatalf("replayed redemption: %v, %v, %v", changed, err, p.RedeemedAt())
	}
	command.EventID = "different"
	if changed, err := p.Redeem(command, replayAt); changed || !errors.Is(err, ErrAlreadyRedeemed) {
		t.Fatalf("different event: %v, %v", changed, err)
	}
}
