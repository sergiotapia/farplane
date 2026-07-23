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

// laneColumnNames is the column list for scanLane (explicit, no abbreviations).
var laneColumnNames = []string{
	"id",
	"project_id",
	"organization_id",
	"owner_user_id",
	"name",
	"lane_kind",
	"lane_template_id",
	"dockerfile_snapshot",
	"image_reference",
	"runtime_kind",
	"runtime_id",
	"agent_provider",
	"agent_provider_session_id",
	"model_source",
	"agent_model",
	"reasoning_effort",
	"status",
	"created_at",
	"updated_at",
}

// laneColumnsSQL builds a SELECT/RETURNING list, optionally table-qualified.
func laneColumnsSQL(tableAlias string) string {
	if tableAlias == "" {
		return strings.Join(laneColumnNames, ", ")
	}
	parts := make([]string, len(laneColumnNames))
	for i, name := range laneColumnNames {
		parts[i] = tableAlias + "." + name
	}
	return strings.Join(parts, ", ")
}

// CreateLaneInput creates a Lane with owner participant in one transaction.
type CreateLaneInput struct {
	ProjectID          *string
	OrganizationID     string
	OwnerUserID        string
	Name               string
	LaneKind           string
	LaneTemplateID     *string
	DockerfileSnapshot string
	ImageReference     *string
	RuntimeKind        string
	RuntimeID          *string
	AgentProvider      string
	ModelSource        string
	AgentModel         string
	ReasoningEffort    *string
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
	kind := strings.TrimSpace(in.LaneKind)
	if kind == "" {
		if in.ProjectID != nil && *in.ProjectID != "" {
			kind = models.LaneKindProject
		} else {
			kind = models.LaneKindScratch
		}
	}
	switch kind {
	case models.LaneKindProject:
		if in.ProjectID == nil || *in.ProjectID == "" {
			return models.Lane{}, fmt.Errorf("project_id is required for project lanes")
		}
	case models.LaneKindScratch:
		in.ProjectID = nil
	default:
		return models.Lane{}, fmt.Errorf("invalid lane_kind")
	}
	name := strings.TrimSpace(in.Name)
	modelSource := strings.TrimSpace(in.ModelSource)
	if modelSource == "" {
		return models.Lane{}, fmt.Errorf("model_source is required")
	}
	agentModel := strings.TrimSpace(in.AgentModel)
	if agentModel == "" {
		return models.Lane{}, fmt.Errorf("agent_model is required")
	}
	var out models.Lane
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := `
			INSERT INTO lanes (
				project_id, organization_id, owner_user_id, name, lane_kind, lane_template_id,
				dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
				model_source, agent_model, reasoning_effort, status, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
			RETURNING ` + laneColumnsSQL("")
		lane, err := scanLane(tx.QueryRow(
			ctx, q,
			in.ProjectID, in.OrganizationID, in.OwnerUserID, name, kind, in.LaneTemplateID,
			in.DockerfileSnapshot, in.ImageReference, in.RuntimeKind, in.RuntimeID, in.AgentProvider,
			modelSource, agentModel, in.ReasoningEffort, in.Status, now,
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

// LaneListGroup is one Project bucket in a grouped lane list.
type LaneListGroup struct {
	ID    string
	Name  string
	Lanes []models.Lane
}

// GroupedLanes is the sidebar list: project groups plus scratch lanes.
type GroupedLanes struct {
	Projects     []LaneListGroup
	ScratchLanes []models.Lane
}

// ListLanesGrouped returns org lanes the user participates in, excluding destroyed.
// One SQL round-trip; sets HasOtherParticipants from a window count.
func (s *Store) ListLanesGrouped(ctx context.Context, organizationID, userID string) (GroupedLanes, error) {
	q := `
		SELECT
			` + laneColumnsSQL("l") + `,
			COALESCE(p.name, '') AS project_name,
			(
				SELECT COUNT(*)::int FROM lane_participants lp WHERE lp.lane_id = l.id
			) > 1 AS has_other_participants
		FROM lanes l
		JOIN lane_participants me ON me.lane_id = l.id AND me.user_id = $2
		LEFT JOIN projects p ON p.id = l.project_id
		WHERE l.organization_id = $1
			AND l.status <> $3
		ORDER BY
			CASE WHEN l.lane_kind = 'scratch' THEN 1 ELSE 0 END,
			lower(COALESCE(p.name, '')),
			l.created_at DESC
	`
	rows, err := s.pool.Query(ctx, q, organizationID, userID, models.LaneStatusDestroyed)
	if err != nil {
		return GroupedLanes{}, fmt.Errorf("list lanes grouped: %w", err)
	}
	defer rows.Close()

	var out GroupedLanes
	projectIndex := map[string]int{}
	for rows.Next() {
		var (
			l           models.Lane
			projectName string
			hasOther    bool
		)
		err := rows.Scan(
			&l.ID, &l.ProjectID, &l.OrganizationID, &l.OwnerUserID, &l.Name, &l.LaneKind,
			&l.LaneTemplateID, &l.DockerfileSnapshot, &l.ImageReference, &l.RuntimeKind,
			&l.RuntimeID, &l.AgentProvider, &l.AgentProviderSessionID,
			&l.ModelSource, &l.AgentModel, &l.ReasoningEffort, &l.Status,
			&l.CreatedAt, &l.UpdatedAt,
			&projectName, &hasOther,
		)
		if err != nil {
			return GroupedLanes{}, err
		}
		l.CreatedAt = l.CreatedAt.UTC()
		l.UpdatedAt = l.UpdatedAt.UTC()
		l.HasOtherParticipants = hasOther
		if l.LaneKind == models.LaneKindScratch || l.ProjectID == nil {
			out.ScratchLanes = append(out.ScratchLanes, l)
			continue
		}
		pid := *l.ProjectID
		idx, ok := projectIndex[pid]
		if !ok {
			out.Projects = append(out.Projects, LaneListGroup{
				ID:   pid,
				Name: projectName,
			})
			idx = len(out.Projects) - 1
			projectIndex[pid] = idx
		}
		out.Projects[idx].Lanes = append(out.Projects[idx].Lanes, l)
	}
	if err := rows.Err(); err != nil {
		return GroupedLanes{}, err
	}
	if out.Projects == nil {
		out.Projects = []LaneListGroup{}
	}
	if out.ScratchLanes == nil {
		out.ScratchLanes = []models.Lane{}
	}
	return out, nil
}

// ListRunningLanesForOrganization returns lanes with a runtime id that are running.
func (s *Store) ListRunningLanesForOrganization(ctx context.Context, organizationID string) ([]models.Lane, error) {
	q := `
		SELECT ` + laneColumnsSQL("") + `
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

// ListLanesForProject returns non-destroyed lanes for a project visible to a participant.
func (s *Store) ListLanesForProject(ctx context.Context, projectID, userID string) ([]models.Lane, error) {
	q := `
		SELECT ` + laneColumnsSQL("l") + `
		FROM lanes l
		JOIN lane_participants p ON p.lane_id = l.id
		WHERE l.project_id = $1 AND p.user_id = $2 AND l.status <> $3
		ORDER BY l.created_at DESC
	`
	rows, err := s.pool.Query(ctx, q, projectID, userID, models.LaneStatusDestroyed)
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
	q := `
		SELECT ` + laneColumnsSQL("") + `
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

// DestroyLane archives a lane as destroyed (owner check is caller's responsibility).
func (s *Store) DestroyLane(ctx context.Context, id string) (models.Lane, error) {
	now := time.Now().UTC()
	q := `
		UPDATE lanes
		SET status = $2, updated_at = $3
		WHERE id = $1 AND status <> $2
		RETURNING ` + laneColumnsSQL("")
	lane, err := scanLane(s.pool.QueryRow(ctx, q, id, models.LaneStatusDestroyed, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Lane{}, ErrNotFound
	}
	if err != nil {
		return models.Lane{}, fmt.Errorf("destroy lane: %w", err)
	}
	return lane, nil
}

// UpdateLaneRuntime updates runtime id, image, and status after create/start.
func (s *Store) UpdateLaneRuntime(ctx context.Context, id string, runtimeID, imageRef *string, status string) (models.Lane, error) {
	now := time.Now().UTC()
	q := `
		UPDATE lanes
		SET runtime_id = COALESCE($2, runtime_id),
			image_reference = COALESCE($3, image_reference),
			status = COALESCE(NULLIF($4, ''), status),
			updated_at = $5
		WHERE id = $1
		RETURNING ` + laneColumnsSQL("")
	lane, err := scanLane(s.pool.QueryRow(ctx, q, id, runtimeID, imageRef, status, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Lane{}, ErrNotFound
	}
	if err != nil {
		return models.Lane{}, fmt.Errorf("update lane runtime: %w", err)
	}
	return lane, nil
}

// UpdateLaneAgentSettingsInput updates agent provider and/or model settings.
type UpdateLaneAgentSettingsInput struct {
	AgentProvider   string
	ModelSource     string
	AgentModel      string
	ReasoningEffort *string
	ClearSession    bool
}

// UpdateLaneAgentSettings updates agent provider, model source, model, and effort.
func (s *Store) UpdateLaneAgentSettings(
	ctx context.Context,
	id string,
	in UpdateLaneAgentSettingsInput,
) (models.Lane, error) {
	now := time.Now().UTC()
	modelSource := strings.TrimSpace(in.ModelSource)
	if modelSource == "" {
		return models.Lane{}, fmt.Errorf("model_source is required")
	}
	agentModel := strings.TrimSpace(in.AgentModel)
	if agentModel == "" {
		return models.Lane{}, fmt.Errorf("agent_model is required")
	}
	q := `
		UPDATE lanes
		SET agent_provider = $2,
			model_source = $3,
			agent_model = $4,
			reasoning_effort = $5,
			agent_provider_session_id = CASE WHEN $6 THEN NULL ELSE agent_provider_session_id END,
			updated_at = $7
		WHERE id = $1
		RETURNING ` + laneColumnsSQL("")
	lane, err := scanLane(s.pool.QueryRow(
		ctx, q, id, in.AgentProvider, modelSource, agentModel, in.ReasoningEffort, in.ClearSession, now,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Lane{}, ErrNotFound
	}
	if err != nil {
		return models.Lane{}, fmt.Errorf("update lane agent settings: %w", err)
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
		SELECT id, lane_id, user_id, role, joined_at
		FROM lane_participants
		WHERE lane_id = $1 AND user_id = $2
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
		&l.ID, &l.ProjectID, &l.OrganizationID, &l.OwnerUserID, &l.Name, &l.LaneKind, &l.LaneTemplateID,
		&l.DockerfileSnapshot, &l.ImageReference, &l.RuntimeKind, &l.RuntimeID, &l.AgentProvider,
		&l.AgentProviderSessionID, &l.ModelSource, &l.AgentModel, &l.ReasoningEffort, &l.Status,
		&l.CreatedAt, &l.UpdatedAt,
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
	err := row.Scan(&p.ID, &p.LaneID, &p.UserID, &p.Role, &p.JoinedAt)
	if err != nil {
		return models.LaneParticipant{}, err
	}
	p.JoinedAt = p.JoinedAt.UTC()
	return p, nil
}
