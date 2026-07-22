package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/farplane/farplane/farplane-backend/internal/lanetemplate"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

const defaultLaneTemplateName = "Farplane default"

// EnsureDefaultLaneTemplate seeds the system default Dockerfile for an org if missing.
func (s *Store) EnsureDefaultLaneTemplate(ctx context.Context, organizationID, createdByUserID string) (models.LaneTemplate, error) {
	existing, err := s.GetSystemDefaultLaneTemplate(ctx, organizationID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return models.LaneTemplate{}, err
	}
	text, err := lanetemplate.DefaultDockerfile()
	if err != nil {
		return models.LaneTemplate{}, fmt.Errorf("default dockerfile: %w", err)
	}
	var createdBy *string
	if createdByUserID != "" {
		createdBy = &createdByUserID
	}
	return s.CreateLaneTemplate(ctx, CreateLaneTemplateInput{
		OrganizationID:  organizationID,
		Name:            defaultLaneTemplateName,
		Description:     "Default Farplane Lane computer with git, build tools, Node, Ruby, and agent CLIs.",
		DockerfileText:  text,
		IsSystemDefault: true,
		CreatedByUserID: createdBy,
	})
}

// GetSystemDefaultLaneTemplate loads the org's system default template.
func (s *Store) GetSystemDefaultLaneTemplate(ctx context.Context, organizationID string) (models.LaneTemplate, error) {
	const q = `
		SELECT id, organization_id, name, description, dockerfile_text, is_system_default,
			forked_from_template_id, created_by_user_id, updated_by_user_id, validation_status,
			validated_image_reference, last_validation_log, validated_at, created_at, updated_at
		FROM lane_templates
		WHERE organization_id = $1 AND is_system_default = true
		ORDER BY created_at ASC
		LIMIT 1
	`
	t, err := scanLaneTemplate(s.pool.QueryRow(ctx, q, organizationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.LaneTemplate{}, ErrNotFound
	}
	if err != nil {
		return models.LaneTemplate{}, fmt.Errorf("get system default lane template: %w", err)
	}
	return t, nil
}

// CreateLaneTemplateInput creates a new template row.
type CreateLaneTemplateInput struct {
	OrganizationID       string
	Name                 string
	Description          string
	DockerfileText       string
	IsSystemDefault      bool
	ForkedFromTemplateID *string
	CreatedByUserID      *string
	LastValidationLog    *string
}

// CreateLaneTemplate inserts a lane template (invalid until Validate build succeeds).
func (s *Store) CreateLaneTemplate(ctx context.Context, in CreateLaneTemplateInput) (models.LaneTemplate, error) {
	now := time.Now().UTC()
	name := strings.TrimSpace(in.Name)
	text := strings.TrimSpace(in.DockerfileText)
	if name == "" || text == "" {
		return models.LaneTemplate{}, fmt.Errorf("name and dockerfile_text are required")
	}
	const q = `
		INSERT INTO lane_templates (
			organization_id, name, description, dockerfile_text, is_system_default,
			forked_from_template_id, created_by_user_id, updated_by_user_id, validation_status,
			last_validation_log, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8,$9,$10,$10)
		RETURNING id, organization_id, name, description, dockerfile_text, is_system_default,
			forked_from_template_id, created_by_user_id, updated_by_user_id, validation_status,
			validated_image_reference, last_validation_log, validated_at, created_at, updated_at
	`
	t, err := scanLaneTemplate(s.pool.QueryRow(
		ctx, q,
		in.OrganizationID, name, in.Description, text, in.IsSystemDefault,
		in.ForkedFromTemplateID, in.CreatedByUserID,
		models.LaneTemplateValidationInvalid, in.LastValidationLog, now,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return models.LaneTemplate{}, fmt.Errorf("template name already exists: %w", err)
		}
		return models.LaneTemplate{}, fmt.Errorf("create lane template: %w", err)
	}
	return t, nil
}

// ListLaneTemplates returns all templates for an organization (seeds default first).
func (s *Store) ListLaneTemplates(ctx context.Context, organizationID, userID string) ([]models.LaneTemplate, error) {
	if _, err := s.EnsureDefaultLaneTemplate(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	const q = `
		SELECT id, organization_id, name, description, dockerfile_text, is_system_default,
			forked_from_template_id, created_by_user_id, updated_by_user_id, validation_status,
			validated_image_reference, last_validation_log, validated_at, created_at, updated_at
		FROM lane_templates
		WHERE organization_id = $1
		ORDER BY is_system_default DESC, name ASC
	`
	rows, err := s.pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list lane templates: %w", err)
	}
	defer rows.Close()
	var out []models.LaneTemplate
	for rows.Next() {
		t, err := scanLaneTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetLaneTemplate loads a template by id.
func (s *Store) GetLaneTemplate(ctx context.Context, id string) (models.LaneTemplate, error) {
	const q = `
		SELECT id, organization_id, name, description, dockerfile_text, is_system_default,
			forked_from_template_id, created_by_user_id, updated_by_user_id, validation_status,
			validated_image_reference, last_validation_log, validated_at, created_at, updated_at
		FROM lane_templates
		WHERE id = $1
	`
	t, err := scanLaneTemplate(s.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.LaneTemplate{}, ErrNotFound
	}
	if err != nil {
		return models.LaneTemplate{}, fmt.Errorf("get lane template: %w", err)
	}
	return t, nil
}

// UpdateLaneTemplateInput patches template fields.
type UpdateLaneTemplateInput struct {
	Name              *string
	Description       *string
	DockerfileText    *string
	UpdatedByUserID   string
	LastValidationLog *string
}

// UpdateLaneTemplate updates a template.
// Changing the Dockerfile after a successful Validate marks the template invalid.
func (s *Store) UpdateLaneTemplate(ctx context.Context, id string, in UpdateLaneTemplateInput) (models.LaneTemplate, error) {
	current, err := s.GetLaneTemplate(ctx, id)
	if err != nil {
		return models.LaneTemplate{}, err
	}
	name := current.Name
	desc := current.Description
	text := current.DockerfileText
	status := current.ValidationStatus
	lintLog := current.LastValidationLog
	clearValidatedImage := false
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		desc = *in.Description
	}
	if in.DockerfileText != nil {
		text = strings.TrimSpace(*in.DockerfileText)
		if text != current.DockerfileText {
			status = models.LaneTemplateValidationInvalid
			clearValidatedImage = true
		}
		if in.LastValidationLog != nil {
			lintLog = in.LastValidationLog
		}
	}
	now := time.Now().UTC()
	const q = `
		UPDATE lane_templates
		SET name = $2, description = $3, dockerfile_text = $4, validation_status = $5,
			last_validation_log = $6, updated_by_user_id = $7, updated_at = $8,
			validated_image_reference = CASE WHEN $9 THEN NULL ELSE validated_image_reference END,
			validated_at = CASE WHEN $9 THEN NULL ELSE validated_at END
		WHERE id = $1
		RETURNING id, organization_id, name, description, dockerfile_text, is_system_default,
			forked_from_template_id, created_by_user_id, updated_by_user_id, validation_status,
			validated_image_reference, last_validation_log, validated_at, created_at, updated_at
	`
	var updatedBy *string
	if in.UpdatedByUserID != "" {
		updatedBy = &in.UpdatedByUserID
	}
	t, err := scanLaneTemplate(s.pool.QueryRow(
		ctx, q, id, name, desc, text, status, lintLog, updatedBy, now, clearValidatedImage,
	))
	if err != nil {
		return models.LaneTemplate{}, fmt.Errorf("update lane template: %w", err)
	}
	return t, nil
}

// ForkLaneTemplate clones a template into a new invalid copy (must Validate again).
func (s *Store) ForkLaneTemplate(ctx context.Context, id, newName, userID string) (models.LaneTemplate, error) {
	src, err := s.GetLaneTemplate(ctx, id)
	if err != nil {
		return models.LaneTemplate{}, err
	}
	name := strings.TrimSpace(newName)
	if name == "" {
		name = src.Name + " (fork)"
	}
	var createdBy *string
	if userID != "" {
		createdBy = &userID
	}
	forkedFrom := src.ID
	return s.CreateLaneTemplate(ctx, CreateLaneTemplateInput{
		OrganizationID:       src.OrganizationID,
		Name:                 name,
		Description:          src.Description,
		DockerfileText:       src.DockerfileText,
		IsSystemDefault:      false,
		ForkedFromTemplateID: &forkedFrom,
		CreatedByUserID:      createdBy,
	})
}

// ErrLaneTemplateInUse is returned when a template still has lanes pointing at it.
var ErrLaneTemplateInUse = errors.New("lane template in use")

// ErrLaneTemplateIsDefault is returned when deleting the system default template.
var ErrLaneTemplateIsDefault = errors.New("lane template is system default")

// LaneTemplatesInUse reports which template ids are referenced by at least one lane.
func (s *Store) LaneTemplatesInUse(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
		SELECT DISTINCT lane_template_id::text
		FROM lanes
		WHERE lane_template_id = ANY($1::uuid[])
	`
	rows, err := s.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("lane templates in use: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("lane templates in use: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// DeleteLaneTemplate removes a template when it is unused and not the system default.
func (s *Store) DeleteLaneTemplate(ctx context.Context, id string) error {
	t, err := s.GetLaneTemplate(ctx, id)
	if err != nil {
		return err
	}
	if t.IsSystemDefault {
		return ErrLaneTemplateIsDefault
	}
	inUse, err := s.LaneTemplatesInUse(ctx, []string{id})
	if err != nil {
		return err
	}
	if inUse[id] {
		return ErrLaneTemplateInUse
	}
	const q = `DELETE FROM lane_templates WHERE id = $1`
	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete lane template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CompleteLaneTemplateValidation stores build result as valid or invalid.
func (s *Store) CompleteLaneTemplateValidation(ctx context.Context, id string, ok bool, imageRef, logText string) (models.LaneTemplate, error) {
	now := time.Now().UTC()
	status := models.LaneTemplateValidationInvalid
	var validatedAt *time.Time
	var image *string
	if ok {
		status = models.LaneTemplateValidationValid
		validatedAt = &now
		image = &imageRef
	}
	const q = `
		UPDATE lane_templates
		SET validation_status = $2,
			validated_image_reference = $3,
			last_validation_log = $4,
			validated_at = $5,
			updated_at = $6
		WHERE id = $1
		RETURNING id, organization_id, name, description, dockerfile_text, is_system_default,
			forked_from_template_id, created_by_user_id, updated_by_user_id, validation_status,
			validated_image_reference, last_validation_log, validated_at, created_at, updated_at
	`
	t, err := scanLaneTemplate(s.pool.QueryRow(ctx, q, id, status, image, logText, validatedAt, now))
	if err != nil {
		return models.LaneTemplate{}, fmt.Errorf("complete lane template validation: %w", err)
	}
	return t, nil
}

func scanLaneTemplate(row scannable) (models.LaneTemplate, error) {
	var t models.LaneTemplate
	err := row.Scan(
		&t.ID, &t.OrganizationID, &t.Name, &t.Description, &t.DockerfileText, &t.IsSystemDefault,
		&t.ForkedFromTemplateID, &t.CreatedByUserID, &t.UpdatedByUserID, &t.ValidationStatus,
		&t.ValidatedImageReference, &t.LastValidationLog, &t.ValidatedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return models.LaneTemplate{}, err
	}
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	if t.ValidatedAt != nil {
		u := t.ValidatedAt.UTC()
		t.ValidatedAt = &u
	}
	return t, nil
}
