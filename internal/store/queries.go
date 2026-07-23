package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/apapangelapeng/action-permission-system/internal/auth"
	"github.com/apapangelapeng/action-permission-system/internal/engine"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// NewID returns a time-sortable id: prefix + hex millis + random suffix.
func NewID(prefix string) string {
	b := make([]byte, 6)
	rand.Read(b)
	return fmt.Sprintf("%s_%011x%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}

type Bot struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type ActionRequest struct {
	ID              string         `json:"id"`
	BotID           string         `json:"bot_id"`
	ActionType      string         `json:"action_type"`
	Payload         map[string]any `json:"payload"`
	Summary         *string        `json:"summary,omitempty"`
	MatchedPolicyID *string        `json:"matched_policy_id,omitempty"`
	Verdict         string         `json:"verdict"`
	Status          string         `json:"status"`
	DecidedBy       *string        `json:"decided_by,omitempty"`
	DecidedAt       *time.Time     `json:"decided_at,omitempty"`
	DecisionNote    *string        `json:"decision_note,omitempty"`
	TTLSeconds      int            `json:"ttl_seconds"`
	ExpiresAt       time.Time      `json:"expires_at"`
	ExecutedAt      *time.Time     `json:"executed_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

const actionRequestCols = `id, bot_id, action_type, payload, summary, matched_policy_id,
	verdict, status, decided_by, decided_at, decision_note, ttl_seconds, expires_at, executed_at, created_at`

func scanActionRequest(row pgx.Row) (*ActionRequest, error) {
	var ar ActionRequest
	err := row.Scan(&ar.ID, &ar.BotID, &ar.ActionType, &ar.Payload, &ar.Summary, &ar.MatchedPolicyID,
		&ar.Verdict, &ar.Status, &ar.DecidedBy, &ar.DecidedAt, &ar.DecisionNote,
		&ar.TTLSeconds, &ar.ExpiresAt, &ar.ExecutedAt, &ar.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &ar, err
}

// ── auth ────────────────────────────────────────────────────────────────────

func (s *Store) BotByAPIKey(ctx context.Context, apiKey string) (*Bot, error) {
	var b Bot
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, disabled_at IS NOT NULL FROM bots WHERE api_key_hash = $1`,
		auth.HashSecret(apiKey),
	).Scan(&b.ID, &b.Name, &b.Disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}

// UserByCredentials verifies the password inside Postgres via pgcrypto crypt().
func (s *Store) UserByCredentials(ctx context.Context, username, password string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, username FROM users
		 WHERE username = $1 AND disabled_at IS NULL AND password_hash = crypt($2, password_hash)`,
		username, password,
	).Scan(&u.ID, &u.Name, &u.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (token string, expiresAt time.Time, err error) {
	plain, hash := auth.NewSessionToken()
	expiresAt = time.Now().Add(ttl).UTC()
	_, err = s.pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hash, userID, expiresAt)
	return plain, expiresAt, err
}

func (s *Store) UserBySessionToken(ctx context.Context, token string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.name, u.username FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now() AND u.disabled_at IS NULL`,
		auth.HashSecret(token),
	).Scan(&u.ID, &u.Name, &u.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// ── policies & settings ─────────────────────────────────────────────────────

func (s *Store) ActivePoliciesForTypes(ctx context.Context, candidateKeys []string) ([]engine.Policy, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, action_type_pattern, matcher_type, matcher_config, effect, priority, version
		 FROM policies WHERE status = 'active' AND action_type_pattern = ANY($1)
		 ORDER BY priority, created_at`, candidateKeys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []engine.Policy
	for rows.Next() {
		var p engine.Policy
		if err := rows.Scan(&p.ID, &p.Name, &p.ActionTypePattern, &p.MatcherType,
			&p.MatcherConfig, &p.Effect, &p.Priority, &p.Version); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// BoolSetting fails closed: on any error the fallback is returned.
func (s *Store) BoolSetting(ctx context.Context, key string, fallback bool) bool {
	var raw []byte
	if err := s.pool.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = $1`, key).Scan(&raw); err != nil {
		return fallback
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return fallback
	}
	return v
}

func (s *Store) IntSetting(ctx context.Context, key string, fallback int) int {
	var raw []byte
	if err := s.pool.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = $1`, key).Scan(&raw); err != nil {
		return fallback
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		return fallback
	}
	return v
}

// ── action requests ─────────────────────────────────────────────────────────

func (s *Store) CreateActionRequest(ctx context.Context, ar *ActionRequest) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO action_requests
		   (id, bot_id, action_type, payload, summary, matched_policy_id, verdict, status, ttl_seconds, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		ar.ID, ar.BotID, ar.ActionType, ar.Payload, ar.Summary, ar.MatchedPolicyID,
		ar.Verdict, ar.Status, ar.TTLSeconds, ar.ExpiresAt)
	return err
}

func (s *Store) ActionRequest(ctx context.Context, id string) (*ActionRequest, error) {
	return scanActionRequest(s.pool.QueryRow(ctx,
		`SELECT `+actionRequestCols+` FROM action_requests WHERE id = $1`, id))
}

// DecideAction is the first-decision-wins transition: the UPDATE only fires
// while the row is still pending and unexpired. won=false means someone (or
// the sweeper) got there first — callers report the existing outcome.
func (s *Store) DecideAction(ctx context.Context, id, userID, newStatus string, note *string) (ar *ActionRequest, won bool, err error) {
	ar, err = scanActionRequest(s.pool.QueryRow(ctx,
		`UPDATE action_requests
		 SET status = $2, decided_by = $3, decided_at = now(), decision_note = $4
		 WHERE id = $1 AND status = 'pending' AND expires_at > now()
		 RETURNING `+actionRequestCols, id, newStatus, userID, note))
	if err == nil {
		return ar, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	ar, err = s.ActionRequest(ctx, id) // lost the race, or never existed
	return ar, false, err
}

// MarkExecuted consumes an approval (or auto-allow): only approved or
// auto_allowed rows within their TTL can transition, and only once.
func (s *Store) MarkExecuted(ctx context.Context, id, botID string, success bool) (ar *ActionRequest, won bool, err error) {
	newStatus := "executed"
	if !success {
		newStatus = "failed"
	}
	ar, err = scanActionRequest(s.pool.QueryRow(ctx,
		`UPDATE action_requests SET status = $3, executed_at = now()
		 WHERE id = $1 AND bot_id = $2 AND status IN ('approved', 'auto_allowed') AND expires_at > now()
		 RETURNING `+actionRequestCols, id, botID, newStatus))
	if err == nil {
		return ar, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	ar, err = s.ActionRequest(ctx, id)
	return ar, false, err
}

// ExpiredRequest is what the sweeper needs to follow up on an expiry —
// enough to reject the linked policy when a proposal timed out.
type ExpiredRequest struct {
	ID         string
	ActionType string
	Payload    map[string]any
}

// ExpireOverdue fails closed on time: pending requests nobody answered and
// approvals the bot never consumed both become 'expired'.
func (s *Store) ExpireOverdue(ctx context.Context) ([]ExpiredRequest, error) {
	rows, err := s.pool.Query(ctx,
		`UPDATE action_requests SET status = 'expired'
		 WHERE status IN ('pending', 'approved') AND expires_at <= now()
		 RETURNING id, action_type, payload`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ExpiredRequest
	for rows.Next() {
		var e ExpiredRequest
		if err := rows.Scan(&e.ID, &e.ActionType, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── audit ───────────────────────────────────────────────────────────────────

// Audit appends an event. Audit failures are logged, never fatal — a decision
// that happened must not be rolled back because telemetry hiccuped.
func (s *Store) Audit(ctx context.Context, actorKind string, actorID *string, eventType, subjectType, subjectID string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_events (actor_kind, actor_id, event_type, subject_type, subject_id, details)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		actorKind, actorID, eventType, subjectType, subjectID, details)
	if err != nil {
		log.Printf("audit write failed (%s %s %s): %v", eventType, subjectType, subjectID, err)
	}
}
