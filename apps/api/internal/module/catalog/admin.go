package catalog

import (
	"context"
	"database/sql"
	"strings"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
)

type Product struct {
	Name string `json:"name"`
}

func SaveProduct(ctx context.Context, tx *sql.Tx, store, id int64, input Product) (int64, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 200 {
		return 0, dbtx.ErrInvalid
	}
	query := "INSERT INTO products (name,store_id) VALUES ($1,$2) RETURNING id"
	args := []any{name, store}
	if id != 0 {
		query = "UPDATE products SET name=$1 WHERE store_id=$2 AND id=$3 RETURNING id"
		args = append(args, id)
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	return id, err
}

type SKU struct {
	Name    string `json:"name"`
	Product int64  `json:"product_id"`
	Price   *int64 `json:"unit_price_cents"`
}

func SaveSKU(ctx context.Context, tx *sql.Tx, store, id int64, input SKU) (int64, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 200 || input.Price == nil || *input.Price < 0 {
		return 0, dbtx.ErrInvalid
	}
	query := "INSERT INTO skus (name,store_id,product_id,unit_price_cents) VALUES ($1,$2,$3,$4) RETURNING id"
	args := []any{name, store, input.Product, *input.Price}
	if id != 0 {
		query = "UPDATE skus SET name=$1,unit_price_cents=$4 WHERE store_id=$2 AND product_id=$3 AND id=$5 RETURNING id"
		args = append(args, id)
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	return id, err
}
