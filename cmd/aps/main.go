package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/apapangelapeng/action-permission-system/internal/api"
	"github.com/apapangelapeng/action-permission-system/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	addr := envOr("APS_ADDR", ":8080")
	dbURL := envOr("APS_DATABASE_URL", "postgres://aps:aps@localhost:5432/aps")

	ctx := context.Background()
	pool, err := connectWithRetry(ctx, dbURL, 30*time.Second)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	if os.Getenv("APS_SEED") == "1" {
		seeded, err := store.SeedIfEmpty(ctx, pool)
		if err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		if seeded {
			log.Println("seeded demo data (users alice/bob, bot demo-bot, policies, action history)")
		}
	}

	log.Printf("aps listening on %s", addr)
	return http.ListenAndServe(addr, api.NewRouter(pool))
}

// connectWithRetry waits for postgres to accept connections — in docker
// compose the app can win the race against the db's first boot.
func connectWithRetry(ctx context.Context, url string, timeout time.Duration) (*pgxpool.Pool, error) {
	deadline := time.Now().Add(timeout)
	for {
		pool, err := pgxpool.New(ctx, url)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(time.Second)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
