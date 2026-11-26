package merchant

import (
	"context"
	"database/sql"
	"strings"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
)

type Store struct {
	Name string `json:"name"`
	Fee  int64  `json:"delivery_fee_cents"`
}

func SaveStore(ctx context.Context, tx *sql.Tx, id int64, input Store) (int64, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 200 || input.Fee < 0 {
		return 0, dbtx.ErrInvalid
	}
	query := "INSERT INTO stores (name,delivery_fee_cents) VALUES ($1,$2) RETURNING id"
	args := []any{name, input.Fee}
	if id != 0 {
		query = "UPDATE stores SET name=$1,delivery_fee_cents=$2 WHERE id=$3 RETURNING id"
		args = append(args, id)
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	return id, err
}

func SetCircle(ctx context.Context, tx *sql.Tx, store, circle int64, active bool) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO store_service_circles (store_id,circle_id) VALUES ($1,$2)
		ON CONFLICT DO NOTHING`, store, circle)
	return err
}

type Point struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Circle  int64  `json:"circle_id"`
}

func SavePoint(ctx context.Context, tx *sql.Tx, id int64, input Point) (int64, error) {
	name, address := strings.TrimSpace(input.Name), strings.TrimSpace(input.Address)
	if name == "" || address == "" || len(name) > 200 || len(address) > 512 {
		return 0, dbtx.ErrInvalid
	}
	query := "INSERT INTO pickup_points (name,address,circle_id) VALUES ($1,$2,$3) RETURNING id"
	args := []any{name, address, input.Circle}
	if id != 0 {
		query = "UPDATE pickup_points SET name=$1,address=$2 WHERE circle_id=$3 AND id=$4 RETURNING id"
		args = append(args, id)
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	return id, err
}

type Leader struct {
	Name string `json:"display_name"`
}

func SaveLeader(ctx context.Context, tx *sql.Tx, id int64, input Leader) (int64, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 200 {
		return 0, dbtx.ErrInvalid
	}
	var err error
	if id == 0 {
		err = tx.QueryRowContext(ctx, "INSERT INTO leaders (display_name) VALUES ($1) RETURNING id", name).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx, "UPDATE leaders SET display_name=$1 WHERE id=$2 RETURNING id", name, id).Scan(&id)
	}
	return id, err
}

func AssignLeader(ctx context.Context, tx *sql.Tx, point, leader int64, active bool) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO pickup_point_leaders (pickup_point_id,leader_id,active) VALUES ($1,$2,$3)
		ON CONFLICT (pickup_point_id,leader_id) DO UPDATE SET active=EXCLUDED.active`, point, leader, active)
	return err
}

type Account struct {
	Provider string `json:"provider"`
	Number   string `json:"merchant_number"`
	Active   *bool  `json:"active"`
}

func SaveAccount(ctx context.Context, tx *sql.Tx, store, id int64, input Account) (int64, error) {
	var err error
	if id == 0 {
		provider, number := strings.TrimSpace(input.Provider), strings.TrimSpace(input.Number)
		if provider == "" || number == "" || len(provider) > 80 || len(number) > 128 {
			return 0, dbtx.ErrInvalid
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO receiving_accounts (store_id,provider,merchant_number,active)
			VALUES ($1,$2,$3,COALESCE($4,true)) RETURNING id`, store, provider, number, input.Active).Scan(&id)
	} else {
		if input.Active == nil || input.Provider != "" || input.Number != "" {
			return 0, dbtx.ErrInvalid
		}
		err = tx.QueryRowContext(ctx, "UPDATE receiving_accounts SET active=$3 WHERE store_id=$1 AND id=$2 RETURNING id", store, id, input.Active).Scan(&id)
	}
	return id, err
}
