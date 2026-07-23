// Package api wires HTTP handlers: the bot-facing decision API, the human
// decision endpoints, the health check, and the embedded dashboard.
package api

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/apapangelapeng/action-permission-system/internal/store"
	"github.com/apapangelapeng/action-permission-system/web"
)

func NewRouter(st *store.Store) http.Handler {
	h := &handlers{st: st}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := st.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "db": "down"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "db": "up"})
	})

	// humans
	mux.HandleFunc("POST /v1/login", h.login)
	mux.HandleFunc("POST /v1/actions/{id}/decision", h.decideAction)
	mux.HandleFunc("GET /v1/actions", h.listActions)
	mux.HandleFunc("GET /v1/audit", h.listAudit)
	mux.HandleFunc("GET /v1/bots", h.listBots)
	mux.HandleFunc("GET /v1/users", h.listUsers)
	mux.HandleFunc("GET /v1/policies", h.listPolicies)
	mux.HandleFunc("POST /v1/policies", h.createPolicy)
	mux.HandleFunc("POST /v1/policies/{id}/disable", h.disablePolicy)
	mux.HandleFunc("POST /v1/bots/{id}/disable", h.setBotDisabled(true))
	mux.HandleFunc("POST /v1/bots/{id}/enable", h.setBotDisabled(false))
	mux.HandleFunc("GET /v1/system/auto-allow", h.getAutoAllow)
	mux.HandleFunc("PUT /v1/system/auto-allow", h.putAutoAllow)

	// bots
	mux.HandleFunc("POST /v1/actions", h.submitAction)
	mux.HandleFunc("GET /v1/actions/{id}", h.pollAction)
	mux.HandleFunc("POST /v1/actions/{id}/executed", h.reportExecuted)

	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic(err) // embed is broken at build time, not a runtime condition
	}
	mux.Handle("/", http.FileServerFS(dist))

	return mux
}
