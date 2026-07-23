package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/apapangelapeng/action-permission-system/internal/engine"
	"github.com/apapangelapeng/action-permission-system/internal/store"
)

const (
	defaultTTLSeconds = 900
	minTTLSeconds     = 10
	sessionTTL        = 24 * time.Hour
)

type handlers struct {
	st *store.Store
}

// ── auth helpers ────────────────────────────────────────────────────────────

// bot resolves the caller from the X-API-Key header; writes the error itself.
func (h *handlers) bot(w http.ResponseWriter, r *http.Request) *store.Bot {
	key := r.Header.Get("X-API-Key")
	if key == "" {
		writeError(w, http.StatusUnauthorized, "missing X-API-Key header")
		return nil
	}
	b, err := h.st.BotByAPIKey(r.Context(), key)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "unknown API key")
		return nil
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "auth lookup failed")
		return nil
	}
	return b
}

// user resolves the caller from the Authorization: Bearer session token.
func (h *handlers) user(w http.ResponseWriter, r *http.Request) *store.User {
	const prefix = "Bearer "
	hdr := r.Header.Get("Authorization")
	if len(hdr) <= len(prefix) || hdr[:len(prefix)] != prefix {
		writeError(w, http.StatusUnauthorized, "missing Authorization: Bearer <session token>")
		return nil
	}
	u, err := h.st.UserBySessionToken(r.Context(), hdr[len(prefix):])
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid or expired session")
		return nil
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "auth lookup failed")
		return nil
	}
	return u
}

// ── POST /v1/login ──────────────────────────────────────────────────────────

func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	u, err := h.st.UserByCredentials(r.Context(), req.Username, req.Password)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	token, expiresAt, err := h.st.CreateSession(r.Context(), u.ID, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	h.st.Audit(r.Context(), "human", &u.ID, "user.login", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token, "expires_at": expiresAt, "user": u,
	})
}

// ── POST /v1/actions ────────────────────────────────────────────────────────

func (h *handlers) submitAction(w http.ResponseWriter, r *http.Request) {
	bot := h.bot(w, r)
	if bot == nil {
		return
	}
	var req struct {
		Type       string         `json:"type"`
		Payload    map[string]any `json:"payload"`
		Summary    *string        `json:"summary"`
		TTLSeconds int            `json:"ttl_seconds"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}

	ctx := r.Context()
	ttl := req.TTLSeconds
	if ttl == 0 {
		ttl = defaultTTLSeconds
	}
	maxTTL := h.st.IntSetting(ctx, "max_ttl_seconds", 21600)
	ttl = min(max(ttl, minTTLSeconds), maxTTL)

	// A policy proposal stores its pending policy row up front; the human
	// decision on this action activates or rejects it.
	if req.Type == engine.PolicyCreateActionType && !bot.Disabled {
		policyID, ok := h.preparePolicyProposal(w, r, bot, req.Payload)
		if !ok {
			return
		}
		req.Payload["policy_id"] = policyID
	}

	ar := &store.ActionRequest{
		ID:         store.NewID("act"),
		BotID:      bot.ID,
		ActionType: req.Type,
		Payload:    req.Payload,
		Summary:    req.Summary,
		TTLSeconds: ttl,
		ExpiresAt:  time.Now().Add(time.Duration(ttl) * time.Second).UTC(),
	}
	details := map[string]any{}

	if bot.Disabled {
		// Per-bot kill switch: denied before any policy runs — and recorded,
		// because what a disabled bot tried to do is exactly what you want to see.
		ar.Verdict, ar.Status = engine.VerdictDeny, "denied"
		details["bot_disabled"] = true
	} else {
		policies, err := h.st.ActivePoliciesForTypes(ctx, engine.CandidateKeys(req.Type))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "policy lookup failed")
			return
		}
		res := engine.Evaluate(policies, req.Payload)
		ar.Verdict = res.Verdict
		if res.MatchedPolicyID != "" {
			ar.MatchedPolicyID = &res.MatchedPolicyID
			details["policy"] = res.MatchedPolicyID
		} else {
			details["default"] = "fail_closed"
		}
		if len(res.MatcherErrors) > 0 {
			details["matcher_errors"] = res.MatcherErrors
		}
		// Global kill switch: allows are downgraded, denies stay denies.
		if ar.Verdict == engine.VerdictAllow && h.st.BoolSetting(ctx, "auto_allow_suspended", false) {
			ar.Verdict = engine.VerdictRequireApproval
			details["auto_allow_suspended"] = true
		}
		switch ar.Verdict {
		case engine.VerdictAllow:
			ar.Status = "auto_allowed"
		case engine.VerdictDeny:
			ar.Status = "denied"
		default:
			ar.Status = "pending"
		}
	}

	if err := h.st.CreateActionRequest(ctx, ar); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store request")
		return
	}
	h.st.Audit(ctx, "bot", &bot.ID, "action.submitted", "action_request", ar.ID,
		map[string]any{"type": ar.ActionType, "ttl_seconds": ar.TTLSeconds})
	h.st.Audit(ctx, "system", nil, "action."+verdictEvent(ar.Status), "action_request", ar.ID, details)

	// A proposal denied by policy (e.g. an explicit deny on aps.policy.*)
	// never reaches a human — reject its pending policy row too.
	if ar.ActionType == engine.PolicyCreateActionType && ar.Status == "denied" {
		if id := str(ar.Payload["policy_id"]); id != "" {
			if ok, _ := h.st.RejectPolicy(ctx, id); ok {
				h.st.Audit(ctx, "system", nil, "policy.rejected", "policy", id,
					map[string]any{"via_action": ar.ID, "reason": "proposal denied by policy"})
			}
		}
	}

	writeJSON(w, http.StatusCreated, ar)
}

func verdictEvent(status string) string {
	switch status {
	case "auto_allowed":
		return "allowed"
	case "denied":
		return "denied"
	default:
		return "pending"
	}
}

// ── GET /v1/actions/{id} ────────────────────────────────────────────────────

func (h *handlers) pollAction(w http.ResponseWriter, r *http.Request) {
	bot := h.bot(w, r)
	if bot == nil {
		return
	}
	ar, err := h.st.ActionRequest(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && ar.BotID != bot.ID) {
		writeError(w, http.StatusNotFound, "no such request")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, ar)
}

// ── POST /v1/actions/{id}/decision ──────────────────────────────────────────

func (h *handlers) decideAction(w http.ResponseWriter, r *http.Request) {
	user := h.user(w, r)
	if user == nil {
		return
	}
	var req struct {
		Decision string  `json:"decision"`
		Note     *string `json:"note"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var newStatus string
	switch req.Decision {
	case "approve":
		newStatus = "approved"
	case "deny":
		newStatus = "denied"
	default:
		writeError(w, http.StatusBadRequest, `decision must be "approve" or "deny"`)
		return
	}

	ar, won, err := h.st.DecideAction(r.Context(), r.PathValue("id"), user.ID, newStatus, req.Note)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such request")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decision failed")
		return
	}
	if !won {
		// First decision wins — report what already happened instead.
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "already decided",
			"request": ar,
		})
		return
	}
	event := "action.approved"
	if newStatus == "denied" {
		event = "action.denied"
	}
	details := map[string]any{}
	if req.Note != nil {
		details["note"] = *req.Note
	}
	h.st.Audit(r.Context(), "human", &user.ID, event, "action_request", ar.ID, details)

	if ar.ActionType == engine.PolicyCreateActionType {
		h.applyPolicyDecision(r.Context(), ar, user, newStatus == "approved")
	}

	writeJSON(w, http.StatusOK, ar)
}

// ── POST /v1/actions/{id}/executed ──────────────────────────────────────────

func (h *handlers) reportExecuted(w http.ResponseWriter, r *http.Request) {
	bot := h.bot(w, r)
	if bot == nil {
		return
	}
	var req struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	success := req.Success == nil || *req.Success

	ar, won, err := h.st.MarkExecuted(r.Context(), r.PathValue("id"), bot.ID, success)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such request")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if !won {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "not executable — only an unexpired approved or auto_allowed request can be executed, once",
			"request": ar,
		})
		return
	}
	event, details := "action.executed", map[string]any{}
	if !success {
		event = "action.failed"
		details["error"] = req.Error
	}
	h.st.Audit(r.Context(), "bot", &bot.ID, event, "action_request", ar.ID, details)
	writeJSON(w, http.StatusOK, ar)
}

// ── expiry sweeper ──────────────────────────────────────────────────────────

// RunExpirySweeper marks overdue pending/approved requests expired every
// interval until ctx is cancelled. Meant to run as a goroutine from main.
func RunExpirySweeper(ctx context.Context, st *store.Store, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			expired, err := st.ExpireOverdue(ctx)
			if err != nil {
				log.Printf("expiry sweep failed: %v", err)
				continue
			}
			for _, e := range expired {
				st.Audit(ctx, "system", nil, "action.expired", "action_request", e.ID, nil)
				// A proposal nobody reviewed in time is rejected with it.
				if e.ActionType == engine.PolicyCreateActionType {
					if id, _ := e.Payload["policy_id"].(string); id != "" {
						if ok, _ := st.RejectPolicy(ctx, id); ok {
							st.Audit(ctx, "system", nil, "policy.rejected", "policy", id,
								map[string]any{"via_action": e.ID, "reason": "proposal expired"})
						}
					}
				}
			}
			if len(expired) > 0 {
				log.Printf("expired %d overdue request(s)", len(expired))
			}
		}
	}
}

// ── shared JSON helpers ─────────────────────────────────────────────────────

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
