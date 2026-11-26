package community

import (
	"context"
	"database/sql"
	"strings"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
)

type Named struct {
	Name      string `json:"name"`
	Community int64  `json:"community_id"`
}

func Save(ctx context.Context, tx *sql.Tx, resource string, id int64, input Named) (int64, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 200 || id < 0 {
		return 0, dbtx.ErrInvalid
	}
	var query string
	args := []any{name}
	switch resource {
	case "communities":
		query = "INSERT INTO communities (name) VALUES ($1) RETURNING id"
		if id != 0 {
			query = "UPDATE communities SET name=$1 WHERE id=$2 RETURNING id"
		}
	case "circles", "buildings":
		table := "service_circles"
		if resource == "buildings" {
			table = "buildings"
		}
		query = "INSERT INTO " + table + " (name,community_id) VALUES ($1,$2) RETURNING id"
		if id != 0 {
			query = "UPDATE " + table + " SET name=$1 WHERE community_id=$2 AND id=$3 RETURNING id"
		}
		args = append(args, input.Community)
	default:
		return 0, dbtx.ErrInvalid
	}
	if id != 0 {
		args = append(args, id)
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	return id, err
}

func AllowBuilding(ctx context.Context, tx *sql.Tx, circle, building int64, allowed bool) error {
	if !allowed {
		_, err := tx.ExecContext(ctx, "UPDATE service_circle_buildings SET active=false WHERE circle_id=$1 AND building_id=$2", circle, building)
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO service_circle_buildings (circle_id,building_id,community_id)
		VALUES ($1,$2,(SELECT community_id FROM service_circles WHERE id=$1)) ON CONFLICT (circle_id,building_id) DO UPDATE SET active=true`, circle, building)
	return err
}
