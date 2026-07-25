package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Policy lifecycle mutations (milestone 3).

func (s *Store) CreatePolicy(ctx context.Context, p *PolicyRow) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO policies (id, name, description, action_type_pattern, matcher_type, matcher_config,
		                       effect, priority, status, created_by_kind, created_by_id, depth,
		                       approved_by, approved_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
		         CASE WHEN $13::text IS NULL THEN NULL ELSE now() END)`,
		p.ID, p.Name, p.Description, p.ActionTypePattern, p.MatcherType, p.MatcherConfig,
		p.Effect, p.Priority, p.Status, p.CreatedByKind, p.CreatedByID, p.Depth, p.ApprovedBy)
	return err
}

func (s *Store) PolicyByID(ctx context.Context, id string) (*PolicyRow, error) {
	var p PolicyRow
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description, action_type_pattern, matcher_type, matcher_config, effect,
		        priority, status, version, created_by_kind, created_by_id, depth,
		        approved_by, approved_at, created_at
		 FROM policies WHERE id = $1`, id).
		Scan(&p.ID, &p.Name, &p.Description, &p.ActionTypePattern, &p.MatcherType,
			&p.MatcherConfig, &p.Effect, &p.Priority, &p.Status, &p.Version,
			&p.CreatedByKind, &p.CreatedByID, &p.Depth,
			&p.ApprovedBy, &p.ApprovedAt, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ActivatePolicy flips a pending proposal to active, recording the approver.
func (s *Store) ActivatePolicy(ctx context.Context, id, approvedBy string) (bool, error) {
	res, err := s.pool.Exec(ctx,
		`UPDATE policies SET status = 'active', approved_by = $2, approved_at = now()
		 WHERE id = $1 AND status = 'pending_approval'`, id, approvedBy)
	return res.RowsAffected() > 0, err
}

// RejectPolicy marks a pending proposal rejected (human denial or expiry).
func (s *Store) RejectPolicy(ctx context.Context, id string) (bool, error) {
	res, err := s.pool.Exec(ctx,
		`UPDATE policies SET status = 'rejected' WHERE id = $1 AND status = 'pending_approval'`, id)
	return res.RowsAffected() > 0, err
}

// FindEquivalentPolicy returns an active or pending policy that is
// semantically identical — same pattern, matcher, config, and effect. The
// name is deliberately ignored: renaming a rule doesn't make it a new rule.
// Disabled and rejected policies don't count, so retired rules can be
// re-proposed.
func (s *Store) FindEquivalentPolicy(ctx context.Context, pattern, matcherType string, config map[string]any, effect string) (*PolicyRow, error) {
	var p PolicyRow
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, status FROM policies
		 WHERE status IN ('active', 'pending_approval')
		   AND action_type_pattern = $1 AND matcher_type = $2
		   AND matcher_config = $3 AND effect = $4
		 LIMIT 1`, pattern, matcherType, config, effect).
		Scan(&p.ID, &p.Name, &p.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// EnablePolicy re-activates a disabled policy. The enabler is recorded as
// the approver — turning a rule back on is taking responsibility for it.
// Descendants disabled by a cascade stay disabled; re-enable them explicitly.
func (s *Store) EnablePolicy(ctx context.Context, id, userID string) (bool, error) {
	res, err := s.pool.Exec(ctx,
		`UPDATE policies SET status = 'active', approved_by = $2, approved_at = now()
		 WHERE id = $1 AND status = 'disabled'`, id, userID)
	return res.RowsAffected() > 0, err
}

// DisablePolicyCascade disables a policy and every bot policy in its
// authorization chain — revoking a rule revokes what it vouched for.
func (s *Store) DisablePolicyCascade(ctx context.Context, id string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`WITH RECURSIVE tree AS (
		   SELECT id FROM policies WHERE id = $1
		   UNION ALL
		   SELECT p.id FROM policies p JOIN tree t ON p.authorized_by_policy_id = t.id
		 )
		 UPDATE policies SET status = 'disabled'
		 WHERE id IN (SELECT id FROM tree) AND status IN ('active', 'pending_approval')
		 RETURNING id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		ids = append(ids, pid)
	}
	return ids, rows.Err()
}
