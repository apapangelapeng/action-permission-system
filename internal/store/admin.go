package store

import (
	"context"
	"strconv"
	"time"
)

// Read models and mutations backing the dashboard.

type AuditEvent struct {
	ID          int64          `json:"id"`
	Ts          time.Time      `json:"ts"`
	ActorKind   string         `json:"actor_kind"`
	ActorID     *string        `json:"actor_id,omitempty"`
	EventType   string         `json:"event_type"`
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id"`
	Details     map[string]any `json:"details"`
}

type BotInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

type PolicyRow struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	ActionTypePattern string         `json:"action_type_pattern"`
	MatcherType       string         `json:"matcher_type"`
	MatcherConfig     map[string]any `json:"matcher_config"`
	Effect            string         `json:"effect"`
	Priority          int            `json:"priority"`
	Status            string         `json:"status"`
	Version           int            `json:"version"`
	CreatedByKind     string         `json:"created_by_kind"`
	CreatedByID       string         `json:"created_by_id"`
	Depth             int            `json:"depth"`
	ApprovedBy        *string        `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time     `json:"approved_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

func (s *Store) ListActionRequests(ctx context.Context, status string, limit int) ([]*ActionRequest, error) {
	q := `SELECT ` + actionRequestCols + ` FROM action_requests`
	args := []any{}
	if status != "" {
		q += ` WHERE status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT ` + itoa(limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*ActionRequest{}
	for rows.Next() {
		ar, err := scanActionRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ar)
	}
	return out, rows.Err()
}

func (s *Store) ListAuditEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ts, actor_kind, actor_id, event_type, subject_type, subject_id, details
		 FROM audit_events ORDER BY ts DESC, id DESC LIMIT `+itoa(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.Ts, &e.ActorKind, &e.ActorID,
			&e.EventType, &e.SubjectType, &e.SubjectID, &e.Details); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListBots(ctx context.Context) ([]BotInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, disabled_at IS NOT NULL, created_at FROM bots ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BotInfo{}
	for rows.Next() {
		var b BotInfo
		if err := rows.Scan(&b.ID, &b.Name, &b.Disabled, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SetBotDisabled flips the per-bot kill switch. found=false → no such bot.
func (s *Store) SetBotDisabled(ctx context.Context, id string, disabled bool) (found bool, err error) {
	var tag string
	if disabled {
		tag = `UPDATE bots SET disabled_at = now() WHERE id = $1 AND disabled_at IS NULL`
	} else {
		tag = `UPDATE bots SET disabled_at = NULL WHERE id = $1`
	}
	res, err := s.pool.Exec(ctx, tag, id)
	if err != nil {
		return false, err
	}
	if res.RowsAffected() > 0 {
		return true, nil
	}
	// Row may exist already in the requested state — still "found".
	var exists bool
	err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bots WHERE id = $1)`, id).Scan(&exists)
	return exists, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, username FROM users WHERE disabled_at IS NULL ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Username); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListPolicies(ctx context.Context) ([]PolicyRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, action_type_pattern, matcher_type, matcher_config, effect,
		        priority, status, version, created_by_kind, created_by_id, depth,
		        approved_by, approved_at, created_at
		 FROM policies ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PolicyRow{}
	for rows.Next() {
		var p PolicyRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.ActionTypePattern, &p.MatcherType,
			&p.MatcherConfig, &p.Effect, &p.Priority, &p.Status, &p.Version,
			&p.CreatedByKind, &p.CreatedByID, &p.Depth,
			&p.ApprovedBy, &p.ApprovedAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) SetSetting(ctx context.Context, key string, value any, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO system_settings (key, value, updated_by, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_by = $3, updated_at = now()`,
		key, value, userID)
	return err
}

// itoa clamps a caller-supplied limit into a safe range for interpolation.
func itoa(n int) string {
	if n <= 0 || n > 1000 {
		n = 100
	}
	return strconv.Itoa(n)
}
