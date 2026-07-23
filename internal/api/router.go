// Package api wires HTTP handlers. Milestone 0 exposes only the health check
// and the embedded dashboard; /v1 action, policy, and decision endpoints
// arrive in milestones 1–3.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/apapangelapeng/action-permission-system/web"
)

func NewRouter(pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := "ok"
		db := "up"
		code := http.StatusOK
		if err := pool.Ping(ctx); err != nil {
			status, db, code = "degraded", "down", http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"status": status, "db": db})
	})

	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic(err) // embed is broken at build time, not a runtime condition
	}
	mux.Handle("/", http.FileServerFS(dist))

	return mux
}
