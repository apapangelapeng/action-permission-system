package api

// Policy lifecycle: humans create/disable directly; bots propose via the
// aps.policy.create action, and the human decision on that action activates
// or rejects the linked policy row.

import (
	"context"
	"net/http"

	"github.com/apapangelapeng/action-permission-system/internal/engine"
	"github.com/apapangelapeng/action-permission-system/internal/store"
)

type policySpec struct {
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	ActionTypePattern string         `json:"action_type_pattern"`
	MatcherType       string         `json:"matcher_type"`
	MatcherConfig     map[string]any `json:"matcher_config"`
	Effect            string         `json:"effect"`
	Priority          int            `json:"priority"`
}

func (sp *policySpec) normalize() {
	if sp.Priority == 0 {
		sp.Priority = 100
	}
	if sp.MatcherConfig == nil {
		sp.MatcherConfig = map[string]any{}
	}
}

func (sp *policySpec) validate() error {
	sp.normalize()
	return engine.ValidatePolicySpec(sp.Name, sp.ActionTypePattern, sp.MatcherType, sp.MatcherConfig, sp.Effect)
}

// ── POST /v1/policies (human: active immediately) ───────────────────────────

func (h *handlers) createPolicy(w http.ResponseWriter, r *http.Request) {
	user := h.user(w, r)
	if user == nil {
		return
	}
	var spec policySpec
	if !readJSON(w, r, &spec) {
		return
	}
	if err := spec.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	p := &store.PolicyRow{
		ID:                store.NewID("pol"),
		Name:              spec.Name,
		Description:       spec.Description,
		ActionTypePattern: spec.ActionTypePattern,
		MatcherType:       spec.MatcherType,
		MatcherConfig:     spec.MatcherConfig,
		Effect:            spec.Effect,
		Priority:          spec.Priority,
		Status:            "active", // humans are the root of authority: no self-approval ceremony
		CreatedByKind:     "human",
		CreatedByID:       user.ID,
		Depth:             0,
		ApprovedBy:        &user.ID,
	}
	if err := h.st.CreatePolicy(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store policy")
		return
	}
	h.st.Audit(r.Context(), "human", &user.ID, "policy.created", "policy", p.ID,
		map[string]any{"effect": p.Effect, "pattern": p.ActionTypePattern})
	h.st.Audit(r.Context(), "human", &user.ID, "policy.activated", "policy", p.ID, nil)
	writeJSON(w, http.StatusCreated, p)
}

// ── POST /v1/policies/{id}/disable (cascades to bot descendants) ────────────

func (h *handlers) disablePolicy(w http.ResponseWriter, r *http.Request) {
	user := h.user(w, r)
	if user == nil {
		return
	}
	id := r.PathValue("id")
	if _, err := h.st.PolicyByID(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "no such policy")
		return
	}
	ids, err := h.st.DisablePolicyCascade(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "disable failed")
		return
	}
	for _, pid := range ids {
		details := map[string]any{}
		if pid != id {
			details["cascaded_from"] = id
		}
		h.st.Audit(r.Context(), "human", &user.ID, "policy.disabled", "policy", pid, details)
	}
	writeJSON(w, http.StatusOK, map[string]any{"disabled": ids})
}

// ── bot proposals: hooks into the action flow ───────────────────────────────

// preparePolicyProposal validates an aps.policy.create submission and stores
// the pending policy row. Returns the policy id to link into the action
// payload, or writes the HTTP error itself.
func (h *handlers) preparePolicyProposal(w http.ResponseWriter, r *http.Request, bot *store.Bot, payload map[string]any) (string, bool) {
	spec := policySpec{
		Name:              str(payload["name"]),
		Description:       str(payload["description"]),
		ActionTypePattern: str(payload["action_type_pattern"]),
		MatcherType:       str(payload["matcher_type"]),
		Effect:            str(payload["effect"]),
	}
	if cfg, ok := payload["matcher_config"].(map[string]any); ok {
		spec.MatcherConfig = cfg
	}
	if pr, ok := payload["priority"].(float64); ok {
		spec.Priority = int(pr)
	}
	if err := spec.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid policy proposal: "+err.Error())
		return "", false
	}

	depth := 1 // human-authored is 0; a bot proposal chains one level down
	if limit := h.st.IntSetting(r.Context(), "policy_depth_limit", 3); depth > limit {
		writeError(w, http.StatusBadRequest, "policy depth limit reached")
		return "", false
	}

	p := &store.PolicyRow{
		ID:                store.NewID("pol"),
		Name:              spec.Name,
		Description:       spec.Description,
		ActionTypePattern: spec.ActionTypePattern,
		MatcherType:       spec.MatcherType,
		MatcherConfig:     spec.MatcherConfig,
		Effect:            spec.Effect,
		Priority:          spec.Priority,
		Status:            "pending_approval",
		CreatedByKind:     "bot",
		CreatedByID:       bot.ID,
		Depth:             depth,
	}
	if err := h.st.CreatePolicy(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store proposed policy")
		return "", false
	}
	h.st.Audit(r.Context(), "bot", &bot.ID, "policy.proposed", "policy", p.ID,
		map[string]any{"effect": p.Effect, "pattern": p.ActionTypePattern})
	return p.ID, true
}

// applyPolicyDecision runs after a human decides an aps.policy.create action:
// approving the action activates the linked policy, denying rejects it.
func (h *handlers) applyPolicyDecision(ctx context.Context, ar *store.ActionRequest, user *store.User, approved bool) {
	policyID := str(ar.Payload["policy_id"])
	if policyID == "" {
		return
	}
	if approved {
		if ok, _ := h.st.ActivatePolicy(ctx, policyID, user.ID); ok {
			h.st.Audit(ctx, "human", &user.ID, "policy.activated", "policy", policyID,
				map[string]any{"via_action": ar.ID})
		}
		return
	}
	if ok, _ := h.st.RejectPolicy(ctx, policyID); ok {
		h.st.Audit(ctx, "human", &user.ID, "policy.rejected", "policy", policyID,
			map[string]any{"via_action": ar.ID})
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
