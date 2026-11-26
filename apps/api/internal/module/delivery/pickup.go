// Package delivery owns fulfillment and pickup verification.
package delivery

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"
)

var (
	ErrPickupDenied    = errors.New("pickup verification denied")
	ErrAlreadyRedeemed = errors.New("order already redeemed")
)

type Pickup struct {
	OrderID       string
	BatchID       string
	PickupPointID string
	LeaderID      string
	ArrivedAt     time.Time
	CodeDigest    [sha256.Size]byte

	eventID    string
	redeemedAt time.Time
}

type Redemption struct {
	EventID       string
	OrderID       string
	PickupPointID string
	LeaderID      string
	Code          string
}

func (p *Pickup) Redeem(command Redemption, now time.Time) (bool, error) {
	digest := sha256.Sum256([]byte(command.Code))
	if command.EventID == "" || command.Code == "" || p.OrderID == "" || p.BatchID == "" ||
		p.PickupPointID == "" || p.LeaderID == "" || command.OrderID != p.OrderID ||
		command.PickupPointID != p.PickupPointID || command.LeaderID != p.LeaderID ||
		p.ArrivedAt.IsZero() || now.Before(p.ArrivedAt) ||
		subtle.ConstantTimeCompare(digest[:], p.CodeDigest[:]) != 1 {
		return false, ErrPickupDenied
	}
	if p.eventID != "" {
		if p.eventID != command.EventID {
			return false, ErrAlreadyRedeemed
		}
		return false, nil
	}

	p.eventID = command.EventID
	p.redeemedAt = now
	return true, nil
}

func (p Pickup) RedeemedAt() time.Time {
	return p.redeemedAt
}
