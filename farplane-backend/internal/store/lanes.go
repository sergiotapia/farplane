package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// ErrConflict is returned for state conflicts (for example turn busy, invalid agent).
var ErrConflict = errors.New("conflict")

// ErrForbidden is returned when the caller lacks permission.
var ErrForbidden = errors.New("forbidden")

// CreateLaneInput creates a Lane with owner participant in one transaction.
type CreateLaneInput struct {
	ProjectID          string
	OrganizationID     string
	OwnerUserID        string
	Name               string
	LaneTemplateID     *string
	DockerfileSnapshot string
	ImageReference           *string
	RuntimeKind        string
	RuntimeID          *string
	AgentProvider      string
	Status             string
}

// CreateLane inserts a lane and owner participant.
func (s *Store) CreateLane(ctx context.Context, in CreateLaneInput) (models.Lane, error) {
	now := time.Now().UTC()
	if in.RuntimeKind == "" {
		in.RuntimeKind = models.RuntimeKindDocker
	}
	if in.Status == "" {
		in.Status = models.LaneStatusCreating
	}
	name := strings.TrimSpace(in.Name)
	var out models.Lane
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		const q = `
			INSERT INTO lanes (
				project_id, organization_id, owner_user_id, name, lane_template_id,
				dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
				status, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
			RETURNING id, project_id, organization_id, owner_user_id, name, lane_template_id,
				dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
				agent_provider_session_id, status, created_at, updated_at
		`
		lane, err := scanLane(tx.QueryRow(
			ctx, q,
			in.ProjectID, in.OrganizationID, in.OwnerUserID, name, in.LaneTemplateID,
			in.DockerfileSnapshot, in.ImageReference, in.RuntimeKind, in.RuntimeID, in.AgentProvider,
			in.Status, now,
		))
		if err != nil {
			return fmt.Errorf("insert lane: %w", err)
		}
		const pq = `
			INSERT INTO lane_participants (lane_id, user_id, role, joined_at)
			VALUES ($1, $2, $3, $4)
		`
		if _, err := tx.Exec(ctx, pq, lane.ID, in.OwnerUserID, models.LaneParticipantRoleOwner, now); err != nil {
			return fmt.Errorf("insert owner participant: %w", err)
		}
		out = lane
		return nil
	})
	if err != nil {
		return models.Lane{}, err
	}
	return out, nil
}

// ListRunningLanesForOrganization returns lanes with a runtime id that are running.
func (s *Store) ListRunningLanesForOrganization(ctx context.Context, organizationID string) ([]models.Lane, error) {
	const q = `
		SELECT id, project_id, organization_id, owner_user_id, name, lane_template_id,
			dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
			agent_provider_session_id, status, created_at, updated_at
		FROM lanes
		WHERE organization_id = $1
			AND runtime_id IS NOT NULL
			AND status = $2
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, q, organizationID, models.LaneStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list running lanes: %w", err)
	}
	defer rows.Close()
	var out []models.Lane
	for rows.Next() {
		lane, err := scanLane(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lane)
	}
	return out, rows.Err()
}

// ListLanesForProject returns lanes for a project visible to an active participant.
func (s *Store) ListLanesForProject(ctx context.Context, projectID, userID string) ([]models.Lane, error) {
	const q = `
		SELECT l.id, l.project_id, l.organization_id, l.owner_user_id, l.name, l.lane_template_id,
			l.dockerfile_snapshot, l.image_reference, l.runtime_kind, l.runtime_id, l.agent_provider,
			l.agent_provider_session_id, l.status, l.created_at, l.updated_at
		FROM lanes l
		JOIN lane_participants p ON p.lane_id = l.id
		WHERE l.project_id = $1 AND p.user_id = $2 AND p.removed_at IS NULL
		ORDER BY l.created_at DESC
	`
	rows, err := s.pool.Query(ctx, q, projectID, userID)
	if err != nil {
		return nil, fmt.Errorf("list lanes: %w", err)
	}
	defer rows.Close()
	var out []models.Lane
	for rows.Next() {
		lane, err := scanLane(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lane)
	}
	return out, rows.Err()
}

// GetLane loads a lane by id.
func (s *Store) GetLane(ctx context.Context, id string) (models.Lane, error) {
	const q = `
		SELECT id, project_id, organization_id, owner_user_id, name, lane_template_id,
			dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
			agent_provider_session_id, status, created_at, updated_at
		FROM lanes
		WHERE id = $1
	`
	lane, err := scanLane(s.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Lane{}, ErrNotFound
	}
	if err != nil {
		return models.Lane{}, fmt.Errorf("get lane: %w", err)
	}
	return lane, nil
}

// UpdateLaneRuntime updates runtime id, image, and status after create/start.
func (s *Store) UpdateLaneRuntime(ctx context.Context, id string, runtimeID, imageRef *string, status string) (models.Lane, error) {
	now := time.Now().UTC()
	const q = `
		UPDATE lanes
		SET runtime_id = COALESCE($2, runtime_id),
			image_reference = COALESCE($3, image_reference),
			status = COALESCE(NULLIF($4, ''), status),
			updated_at = $5
		WHERE id = $1
		RETURNING id, project_id, organization_id, owner_user_id, name, lane_template_id,
			dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
			agent_provider_session_id, status, created_at, updated_at
	`
	lane, err := scanLane(s.pool.QueryRow(ctx, q, id, runtimeID, imageRef, status, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Lane{}, ErrNotFound
	}
	if err != nil {
		return models.Lane{}, fmt.Errorf("update lane runtime: %w", err)
	}
	return lane, nil
}

// UpdateLaneAgentProvider switches the agent (clears provider session).
func (s *Store) UpdateLaneAgentProvider(ctx context.Context, id, agentProvider string) (models.Lane, error) {
	now := time.Now().UTC()
	const q = `
		UPDATE lanes
		SET agent_provider = $2,
			agent_provider_session_id = NULL,
			updated_at = $3
		WHERE id = $1
		RETURNING id, project_id, organization_id, owner_user_id, name, lane_template_id,
			dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
			agent_provider_session_id, status, created_at, updated_at
	`
	lane, err := scanLane(s.pool.QueryRow(ctx, q, id, agentProvider, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Lane{}, ErrNotFound
	}
	if err != nil {
		return models.Lane{}, fmt.Errorf("update lane agent: %w", err)
	}
	return lane, nil
}

// SetLaneAgentProviderSessionID stores the CLI resume id.
func (s *Store) SetLaneAgentProviderSessionID(ctx context.Context, id string, sessionID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE lanes
		SET agent_provider_session_id = $2, updated_at = $3
		WHERE id = $1
	`, id, sessionID, time.Now().UTC())
	return err
}

// RequireActiveLaneParticipant returns the participant or ErrForbidden/ErrNotFound.
func (s *Store) RequireActiveLaneParticipant(ctx context.Context, laneID, userID string) (models.LaneParticipant, error) {
	const q = `
		SELECT id, lane_id, user_id, role, joined_at, removed_at, removed_by_user_id
		FROM lane_participants
		WHERE lane_id = $1 AND user_id = $2 AND removed_at IS NULL
	`
	p, err := scanLaneParticipant(s.pool.QueryRow(ctx, q, laneID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.LaneParticipant{}, ErrForbidden
	}
	if err != nil {
		return models.LaneParticipant{}, fmt.Errorf("require active lane participant: %w", err)
	}
	return p, nil
}

// RequireLaneOwner ensures the user is the active owner participant.
func (s *Store) RequireLaneOwner(ctx context.Context, laneID, userID string) (models.LaneParticipant, error) {
	p, err := s.RequireActiveLaneParticipant(ctx, laneID, userID)
	if err != nil {
		return models.LaneParticipant{}, err
	}
	if p.Role != models.LaneParticipantRoleOwner {
		return models.LaneParticipant{}, ErrForbidden
	}
	return p, nil
}

func scanLane(row scannable) (models.Lane, error) {
	var l models.Lane
	err := row.Scan(
		&l.ID, &l.ProjectID, &l.OrganizationID, &l.OwnerUserID, &l.Name, &l.LaneTemplateID,
		&l.DockerfileSnapshot, &l.ImageReference, &l.RuntimeKind, &l.RuntimeID, &l.AgentProvider,
		&l.AgentProviderSessionID, &l.Status, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return models.Lane{}, err
	}
	l.CreatedAt = l.CreatedAt.UTC()
	l.UpdatedAt = l.UpdatedAt.UTC()
	return l, nil
}

func scanLaneParticipant(row scannable) (models.LaneParticipant, error) {
	var p models.LaneParticipant
	err := row.Scan(&p.ID, &p.LaneID, &p.UserID, &p.Role, &p.JoinedAt, &p.RemovedAt, &p.RemovedByUserID)
	if err != nil {
		return models.LaneParticipant{}, err
	}
	p.JoinedAt = p.JoinedAt.UTC()
	if p.RemovedAt != nil {
		u := p.RemovedAt.UTC()
		p.RemovedAt = &u
	}
	return p, nil
}
