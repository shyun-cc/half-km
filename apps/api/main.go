package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/httpapi"
	"github.com/shyun-cc/half-km/apps/api/internal/workflow"

	"github.com/gogf/gf/v2/net/ghttp"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shyun-cc/shw-gf-lib/response"
)

func main() {
	server := ghttp.GetServer()
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server.SetAddr(addr)
	server.BindHandler("GET:/health", func(r *ghttp.Request) {
		response.Success(r, map[string]string{"status": "ok"})
	})

	dsn := os.Getenv("DATABASE_URL")
	if dsn != "" {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			log.Fatal("could not configure PostgreSQL")
		}
		defer db.Close()
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		auth, err := httpapi.LoadAuth(os.Getenv("HALFKM_AUTH_FILE"))
		if err != nil {
			log.Fatal("could not load HALFKM_AUTH_FILE: ", err)
		}
		httpapi.Bind(server, db, auth)
		workerContext, stopWorker := context.WithCancel(context.Background())
		defer stopWorker()
		go workflow.RunWorker(workerContext, db)
		server.BindHandler("GET:/ready", func(r *ghttp.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			var migrated bool
			err := db.QueryRowContext(ctx, "SELECT to_regclass('coupon_uses') IS NOT NULL").Scan(&migrated)
			if err != nil || !migrated {
				r.Response.WriteHeader(http.StatusServiceUnavailable)
				response.Error(r, "database or schema unavailable", http.StatusServiceUnavailable)
				return
			}
			response.Success(r, map[string]string{"status": "ready"})
		})
	}
	server.Run()
}
