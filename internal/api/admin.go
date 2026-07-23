package api

// Dashboard-facing endpoints: read views plus the two kill switches.
// All require a human session.

import (
	"net/http"
	"strconv"
)

func (h *handlers) listActions(w http.ResponseWriter, r *http.Request) {
	if h.user(w, r) == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 100
	}
	out, err := h.st.ListActionRequests(r.Context(), r.URL.Query().Get("status"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) listAudit(w http.ResponseWriter, r *http.Request) {
	if h.user(w, r) == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 200
	}
	out, err := h.st.ListAuditEvents(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) listBots(w http.ResponseWriter, r *http.Request) {
	if h.user(w, r) == nil {
		return
	}
	out, err := h.st.ListBots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	if h.user(w, r) == nil {
		return
	}
	out, err := h.st.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) listPolicies(w http.ResponseWriter, r *http.Request) {
	if h.user(w, r) == nil {
		return
	}
	out, err := h.st.ListPolicies(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// setBotDisabled backs POST /v1/bots/{id}/disable and .../enable —
// the per-bot kill switch.
func (h *handlers) setBotDisabled(disabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := h.user(w, r)
		if user == nil {
			return
		}
		id := r.PathValue("id")
		found, err := h.st.SetBotDisabled(r.Context(), id, disabled)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "no such bot")
			return
		}
		event := "bot.enabled"
		if disabled {
			event = "bot.disabled"
		}
		h.st.Audit(r.Context(), "human", &user.ID, event, "bot", id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "disabled": disabled})
	}
}

func (h *handlers) getAutoAllow(w http.ResponseWriter, r *http.Request) {
	if h.user(w, r) == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{
		"suspended": h.st.BoolSetting(r.Context(), "auto_allow_suspended", false),
	})
}

// putAutoAllow is the global kill switch: suspended=true sends every action
// back to a human without touching any policy.
func (h *handlers) putAutoAllow(w http.ResponseWriter, r *http.Request) {
	user := h.user(w, r)
	if user == nil {
		return
	}
	var req struct {
		Suspended *bool `json:"suspended"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Suspended == nil {
		writeError(w, http.StatusBadRequest, "suspended (bool) is required")
		return
	}
	if err := h.st.SetSetting(r.Context(), "auto_allow_suspended", *req.Suspended, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	event := "system.auto_allow_restored"
	if *req.Suspended {
		event = "system.auto_allow_suspended"
	}
	h.st.Audit(r.Context(), "human", &user.ID, event, "system", "auto_allow_suspended", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"suspended": *req.Suspended})
}
