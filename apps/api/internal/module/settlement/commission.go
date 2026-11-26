// Package settlement computes the single-level leader commission.
package settlement

import "errors"

const basisPoints = int64(10_000)

// CommissionCents rounds down after applying the frozen rate to the
// net merchandise amount, excluding delivery fees.
func CommissionCents(netMerchandiseCents, rateBPS int64) (int64, error) {
	if netMerchandiseCents < 0 || rateBPS < 0 || rateBPS > basisPoints {
		return 0, errors.New("invalid commission basis or rate")
	}
	whole, remainder := netMerchandiseCents/basisPoints, netMerchandiseCents%basisPoints
	return whole*rateBPS + remainder*rateBPS/basisPoints, nil
}
