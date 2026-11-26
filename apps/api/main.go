package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

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
		server.BindHandler("GET:/ready", func(r *ghttp.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := db.PingContext(ctx); err != nil {
				r.Response.WriteHeader(http.StatusServiceUnavailable)
				response.Error(r, "database unavailable", http.StatusServiceUnavailable)
				return
			}
			response.Success(r, map[string]string{"status": "ready"})
		})
	}
	server.Run()
}
