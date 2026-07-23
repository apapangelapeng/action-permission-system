// Package aps is the bot-side client for the Action Permission System.
//
// The core call is Check: describe the action you want to take, and block
// until the system answers. If the verdict needs a human, Check polls until
// someone decides or the request expires — to the bot, a gated action just
// looks slow. Execute the real action only when Allowed() is true, then
// confirm with ReportExecuted (approvals are single-use).
package aps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	BaseURL      string
	APIKey       string
	HTTPClient   *http.Client
	PollInterval time.Duration
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		HTTPClient:   &http.Client{Timeout: 15 * time.Second},
		PollInterval: 2 * time.Second,
	}
}

// Action describes what the bot wants to do. Type is a dot-namespaced kind
// ("db.query", "http.request"); Payload is the full, honest parameters —
// it is what humans see and what policies inspect. Summary is display-only.
type Action struct {
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
	Summary    string         `json:"summary,omitempty"`
	TTLSeconds int            `json:"ttl_seconds,omitempty"`
}

// Request mirrors the server's view of a submitted action.
type Request struct {
	ID              string         `json:"id"`
	ActionType      string         `json:"action_type"`
	Payload         map[string]any `json:"payload"`
	Verdict         string         `json:"verdict"`
	Status          string         `json:"status"`
	MatchedPolicyID string         `json:"matched_policy_id"`
	DecidedBy       string         `json:"decided_by"`
	DecisionNote    string         `json:"decision_note"`
	ExpiresAt       time.Time      `json:"expires_at"`
	CreatedAt       time.Time      `json:"created_at"`
}

// Allowed reports whether the bot may perform the action now.
func (r *Request) Allowed() bool {
	return r.Status == "auto_allowed" || r.Status == "approved"
}

// Pending reports whether a human still needs to decide.
func (r *Request) Pending() bool { return r.Status == "pending" }

// Submit sends the action for judgment and returns immediately —
// the result may be pending.
func (c *Client) Submit(ctx context.Context, a Action) (*Request, error) {
	return c.do(ctx, http.MethodPost, "/v1/actions", a)
}

// Get fetches the current state of a previously submitted request.
func (c *Client) Get(ctx context.Context, id string) (*Request, error) {
	return c.do(ctx, http.MethodGet, "/v1/actions/"+id, nil)
}

// Check submits the action and blocks until it is no longer pending: a
// policy verdict comes back instantly; a human decision arrives when someone
// clicks; an unanswered request eventually expires (which counts as a no).
func (c *Client) Check(ctx context.Context, a Action) (*Request, error) {
	req, err := c.Submit(ctx, a)
	if err != nil {
		return nil, err
	}
	for req.Pending() {
		select {
		case <-ctx.Done():
			return req, ctx.Err()
		case <-time.After(c.PollInterval):
		}
		if req, err = c.Get(ctx, req.ID); err != nil {
			return nil, err
		}
	}
	return req, nil
}

// ReportExecuted closes the loop after the bot performed an allowed action —
// it consumes the single-use approval. Pass success=false (with an error
// message) if the action was attempted but failed.
func (c *Client) ReportExecuted(ctx context.Context, id string, success bool, errMsg string) (*Request, error) {
	body := map[string]any{"success": success}
	if errMsg != "" {
		body["error"] = errMsg
	}
	return c.do(ctx, http.MethodPost, "/v1/actions/"+id+"/executed", body)
}

// PolicySpec is a rule the bot proposes. It always requires human approval —
// the server refuses any configuration that could change that.
type PolicySpec struct {
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	ActionTypePattern string         `json:"action_type_pattern"`
	MatcherType       string         `json:"matcher_type"`
	MatcherConfig     map[string]any `json:"matcher_config"`
	Effect            string         `json:"effect"`
	Priority          int            `json:"priority,omitempty"`
}

// ProposePolicy submits an aps.policy.create action carrying the spec and
// blocks like Check. If the returned request is Allowed(), the rule is active.
func (c *Client) ProposePolicy(ctx context.Context, spec PolicySpec, summary string) (*Request, error) {
	payload := map[string]any{
		"name":                spec.Name,
		"description":         spec.Description,
		"action_type_pattern": spec.ActionTypePattern,
		"matcher_type":        spec.MatcherType,
		"matcher_config":      spec.MatcherConfig,
		"effect":              spec.Effect,
	}
	if spec.Priority != 0 {
		payload["priority"] = spec.Priority
	}
	return c.Check(ctx, Action{
		Type:       "aps.policy.create",
		Payload:    payload,
		Summary:    summary,
		TTLSeconds: 3600, // humans review rules more slowly than actions
	})
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*Request, error) {
	var buf *bytes.Buffer
	if body != nil {
		buf = &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, err
		}
	} else {
		buf = bytes.NewBuffer(nil)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("X-API-Key", c.APIKey)
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(res.Body).Decode(&e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return nil, fmt.Errorf("aps: %s %s: %s", method, path, e.Error)
	}
	var out Request
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
