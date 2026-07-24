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

// EnsureScratchEnvironment seeds the org Scratch Environment from the Farplane default.
func (s *Store) EnsureScratchEnvironment(ctx context.Context, organizationID, updatedByUserID string) (models.ScratchEnvironment, error) {
	existing, err := s.GetScratchEnvironment(ctx, organizationID)
	if err == nil {
		return existing, nil
	}

	if !errors.Is(err, ErrNotFound) {
		return models.ScratchEnvironment{}, err
	}

	text, err := lanetemplate.DefaultDockerfile()
	if err != nil {
		return models.ScratchEnvironment{}, fmt.Errorf("default dockerfile: %w", err)
	}

	return s.UpsertScratchEnvironment(ctx, UpsertScratchEnvironmentInput{
		OrganizationID:  organizationID,
		DockerfileText:  text,
		UpdatedByUserID: updatedByUserID,
	})
}

// GetScratchEnvironment loads the Scratch Environment for an organization.
func (s *Store) GetScratchEnvironment(ctx context.Context, organizationID string) (models.ScratchEnvironment, error) {
	const q = `
		SELECT organization_id, dockerfile_text, validation_status, validated_image_reference,
			last_validation_log, validated_at, updated_by_user_id, created_at, updated_at
		FROM scratch_environments
		WHERE organization_id = $1
	`

	env, err := scanScratchEnvironment(s.pool.QueryRow(ctx, q, organizationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ScratchEnvironment{}, ErrNotFound
	}

	if err != nil {
		return models.ScratchEnvironment{}, fmt.Errorf("get scratch environment: %w", err)
	}

	return env, nil
}

// UpsertScratchEnvironmentInput creates or replaces Scratch Environment Dockerfile text.
type UpsertScratchEnvironmentInput struct {
	OrganizationID    string
	DockerfileText    string
	UpdatedByUserID   string
	LastValidationLog *string
}

// UpsertScratchEnvironment inserts or updates the Scratch Environment.
// Changing the Dockerfile marks validation invalid.
func (s *Store) UpsertScratchEnvironment(ctx context.Context, in UpsertScratchEnvironmentInput) (models.ScratchEnvironment, error) {
	text := strings.TrimSpace(in.DockerfileText)
	if text == "" {
		return models.ScratchEnvironment{}, errors.New("dockerfile_text is required")
	}

	now := time.Now().UTC()

	var updatedBy *string
	if in.UpdatedByUserID != "" {
		updatedBy = &in.UpdatedByUserID
	}

	const q = `
		INSERT INTO scratch_environments (
			organization_id, dockerfile_text, validation_status, last_validation_log,
			updated_by_user_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (organization_id) DO UPDATE SET
			dockerfile_text = EXCLUDED.dockerfile_text,
			validation_status = $3,
			validated_image_reference = NULL,
			validated_at = NULL,
			last_validation_log = EXCLUDED.last_validation_log,
			updated_by_user_id = EXCLUDED.updated_by_user_id,
			updated_at = EXCLUDED.updated_at
		RETURNING organization_id, dockerfile_text, validation_status, validated_image_reference,
			last_validation_log, validated_at, updated_by_user_id, created_at, updated_at
	`

	env, err := scanScratchEnvironment(s.pool.QueryRow(
		ctx, q,
		in.OrganizationID, text, models.EnvironmentValidationInvalid,
		in.LastValidationLog, updatedBy, now,
	))
	if err != nil {
		return models.ScratchEnvironment{}, fmt.Errorf("upsert scratch environment: %w", err)
	}

	return env, nil
}

// CompleteScratchEnvironmentValidation stores build result as valid or invalid.
//
//nolint:dupl // scratch/project validation updates share the same status transition shape.
func (s *Store) CompleteScratchEnvironmentValidation(
	ctx context.Context, organizationID string, ok bool, imageRef, logText string,
) (models.ScratchEnvironment, error) {
	now := time.Now().UTC()
	status := models.EnvironmentValidationInvalid

	var (
		validatedAt *time.Time
		image       *string
	)

	if ok {
		status = models.EnvironmentValidationValid
		validatedAt = &now
		image = &imageRef
	}

	const q = `
		UPDATE scratch_environments
		SET validation_status = $2,
			validated_image_reference = $3,
			last_validation_log = $4,
			validated_at = $5,
			updated_at = $6
		WHERE organization_id = $1
		RETURNING organization_id, dockerfile_text, validation_status, validated_image_reference,
			last_validation_log, validated_at, updated_by_user_id, created_at, updated_at
	`

	env, err := scanScratchEnvironment(s.pool.QueryRow(
		ctx, q, organizationID, status, image, logText, validatedAt, now,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ScratchEnvironment{}, ErrNotFound
	}

	if err != nil {
		return models.ScratchEnvironment{}, fmt.Errorf("complete scratch environment validation: %w", err)
	}

	return env, nil
}

// GetProjectEnvironment loads the Project Environment when present.
func (s *Store) GetProjectEnvironment(ctx context.Context, projectID string) (models.ProjectEnvironment, error) {
	const q = `
		SELECT project_id, organization_id, dockerfile_text, validation_status,
			validated_image_reference, last_validation_log, validated_at,
			generation_status, generation_log, updated_by_user_id, created_at, updated_at
		FROM project_environments
		WHERE project_id = $1
	`

	env, err := scanProjectEnvironment(s.pool.QueryRow(ctx, q, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ProjectEnvironment{}, ErrNotFound
	}

	if err != nil {
		return models.ProjectEnvironment{}, fmt.Errorf("get project environment: %w", err)
	}

	return env, nil
}

// UpsertProjectEnvironmentInput creates or replaces a Project Environment Dockerfile.
type UpsertProjectEnvironmentInput struct {
	ProjectID         string
	OrganizationID    string
	DockerfileText    string
	UpdatedByUserID   string
	LastValidationLog *string
	GenerationStatus  string
	GenerationLog     *string
}

// UpsertProjectEnvironment inserts or updates the Project Environment.
func (s *Store) UpsertProjectEnvironment(ctx context.Context, in UpsertProjectEnvironmentInput) (models.ProjectEnvironment, error) {
	text := strings.TrimSpace(in.DockerfileText)
	if text == "" {
		return models.ProjectEnvironment{}, errors.New("dockerfile_text is required")
	}

	generationStatus := strings.TrimSpace(in.GenerationStatus)
	if generationStatus == "" {
		generationStatus = models.EnvironmentGenerationIdle
	}

	now := time.Now().UTC()

	var updatedBy *string
	if in.UpdatedByUserID != "" {
		updatedBy = &in.UpdatedByUserID
	}

	const q = `
		INSERT INTO project_environments (
			project_id, organization_id, dockerfile_text, validation_status,
			last_validation_log, generation_status, generation_log,
			updated_by_user_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		ON CONFLICT (project_id) DO UPDATE SET
			dockerfile_text = EXCLUDED.dockerfile_text,
			validation_status = $4,
			validated_image_reference = NULL,
			validated_at = NULL,
			last_validation_log = EXCLUDED.last_validation_log,
			generation_status = EXCLUDED.generation_status,
			generation_log = EXCLUDED.generation_log,
			updated_by_user_id = EXCLUDED.updated_by_user_id,
			updated_at = EXCLUDED.updated_at
		RETURNING project_id, organization_id, dockerfile_text, validation_status,
			validated_image_reference, last_validation_log, validated_at,
			generation_status, generation_log, updated_by_user_id, created_at, updated_at
	`

	env, err := scanProjectEnvironment(s.pool.QueryRow(
		ctx, q,
		in.ProjectID, in.OrganizationID, text, models.EnvironmentValidationInvalid,
		in.LastValidationLog, generationStatus, in.GenerationLog, updatedBy, now,
	))
	if err != nil {
		return models.ProjectEnvironment{}, fmt.Errorf("upsert project environment: %w", err)
	}

	return env, nil
}

// MarkProjectEnvironmentGenerating sets generation_status=generating.
// Creates a row with the Farplane default Dockerfile when none exists.
func (s *Store) MarkProjectEnvironmentGenerating(
	ctx context.Context, projectID, organizationID, updatedByUserID string,
) (models.ProjectEnvironment, error) {
	existing, err := s.GetProjectEnvironment(ctx, projectID)
	if err == nil {
		now := time.Now().UTC()

		var updatedBy *string
		if updatedByUserID != "" {
			updatedBy = &updatedByUserID
		}

		const q = `
			UPDATE project_environments
			SET generation_status = $2,
				generation_log = NULL,
				updated_by_user_id = $3,
				updated_at = $4
			WHERE project_id = $1
			RETURNING project_id, organization_id, dockerfile_text, validation_status,
				validated_image_reference, last_validation_log, validated_at,
				generation_status, generation_log, updated_by_user_id, created_at, updated_at
		`

		env, err := scanProjectEnvironment(s.pool.QueryRow(
			ctx, q, projectID, models.EnvironmentGenerationGenerating, updatedBy, now,
		))
		if err != nil {
			return models.ProjectEnvironment{}, fmt.Errorf("mark project environment generating: %w", err)
		}

		return env, nil
	}

	if !errors.Is(err, ErrNotFound) {
		return models.ProjectEnvironment{}, err
	}

	_ = existing

	text, err := lanetemplate.DefaultDockerfile()
	if err != nil {
		return models.ProjectEnvironment{}, fmt.Errorf("default dockerfile: %w", err)
	}

	return s.UpsertProjectEnvironment(ctx, UpsertProjectEnvironmentInput{
		ProjectID:        projectID,
		OrganizationID:   organizationID,
		DockerfileText:   text,
		UpdatedByUserID:  updatedByUserID,
		GenerationStatus: models.EnvironmentGenerationGenerating,
	})
}

// CompleteProjectEnvironmentGeneration stores generator output.
func (s *Store) CompleteProjectEnvironmentGeneration(
	ctx context.Context,
	projectID string,
	ok bool,
	dockerfileText, generationLog string,
	updatedByUserID string,
) (models.ProjectEnvironment, error) {
	now := time.Now().UTC()

	var updatedBy *string
	if updatedByUserID != "" {
		updatedBy = &updatedByUserID
	}

	if !ok {
		const q = `
			UPDATE project_environments
			SET generation_status = $2,
				generation_log = $3,
				updated_by_user_id = $4,
				updated_at = $5
			WHERE project_id = $1
			RETURNING project_id, organization_id, dockerfile_text, validation_status,
				validated_image_reference, last_validation_log, validated_at,
				generation_status, generation_log, updated_by_user_id, created_at, updated_at
		`

		env, err := scanProjectEnvironment(s.pool.QueryRow(
			ctx, q, projectID, models.EnvironmentGenerationFailed, generationLog, updatedBy, now,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ProjectEnvironment{}, ErrNotFound
		}

		if err != nil {
			return models.ProjectEnvironment{}, fmt.Errorf("complete project environment generation: %w", err)
		}

		return env, nil
	}

	text := strings.TrimSpace(dockerfileText)
	if text == "" {
		return models.ProjectEnvironment{}, errors.New("dockerfile_text is required")
	}

	const q = `
		UPDATE project_environments
		SET dockerfile_text = $2,
			validation_status = $3,
			validated_image_reference = NULL,
			validated_at = NULL,
			generation_status = $4,
			generation_log = $5,
			updated_by_user_id = $6,
			updated_at = $7
		WHERE project_id = $1
		RETURNING project_id, organization_id, dockerfile_text, validation_status,
			validated_image_reference, last_validation_log, validated_at,
			generation_status, generation_log, updated_by_user_id, created_at, updated_at
	`

	env, err := scanProjectEnvironment(s.pool.QueryRow(
		ctx, q,
		projectID, text, models.EnvironmentValidationInvalid,
		models.EnvironmentGenerationIdle, generationLog, updatedBy, now,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ProjectEnvironment{}, ErrNotFound
	}

	if err != nil {
		return models.ProjectEnvironment{}, fmt.Errorf("complete project environment generation: %w", err)
	}

	return env, nil
}

// CompleteProjectEnvironmentValidation stores build result as valid or invalid.
//
//nolint:dupl // scratch/project validation updates share the same status transition shape.
func (s *Store) CompleteProjectEnvironmentValidation(
	ctx context.Context, projectID string, ok bool, imageRef, logText string,
) (models.ProjectEnvironment, error) {
	now := time.Now().UTC()
	status := models.EnvironmentValidationInvalid

	var (
		validatedAt *time.Time
		image       *string
	)

	if ok {
		status = models.EnvironmentValidationValid
		validatedAt = &now
		image = &imageRef
	}

	const q = `
		UPDATE project_environments
		SET validation_status = $2,
			validated_image_reference = $3,
			last_validation_log = $4,
			validated_at = $5,
			updated_at = $6
		WHERE project_id = $1
		RETURNING project_id, organization_id, dockerfile_text, validation_status,
			validated_image_reference, last_validation_log, validated_at,
			generation_status, generation_log, updated_by_user_id, created_at, updated_at
	`

	env, err := scanProjectEnvironment(s.pool.QueryRow(
		ctx, q, projectID, status, image, logText, validatedAt, now,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ProjectEnvironment{}, ErrNotFound
	}

	if err != nil {
		return models.ProjectEnvironment{}, fmt.Errorf("complete project environment validation: %w", err)
	}

	return env, nil
}

// DeleteProjectEnvironment removes the Project Environment row.
func (s *Store) DeleteProjectEnvironment(ctx context.Context, projectID string) error {
	const q = `DELETE FROM project_environments WHERE project_id = $1`

	tag, err := s.pool.Exec(ctx, q, projectID)
	if err != nil {
		return fmt.Errorf("delete project environment: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func scanScratchEnvironment(row scannable) (models.ScratchEnvironment, error) {
	var env models.ScratchEnvironment

	err := row.Scan(
		&env.OrganizationID, &env.DockerfileText, &env.ValidationStatus,
		&env.ValidatedImageReference, &env.LastValidationLog, &env.ValidatedAt,
		&env.UpdatedByUserID, &env.CreatedAt, &env.UpdatedAt,
	)
	if err != nil {
		return models.ScratchEnvironment{}, err
	}

	env.CreatedAt = env.CreatedAt.UTC()

	env.UpdatedAt = env.UpdatedAt.UTC()
	if env.ValidatedAt != nil {
		u := env.ValidatedAt.UTC()
		env.ValidatedAt = &u
	}

	return env, nil
}

func scanProjectEnvironment(row scannable) (models.ProjectEnvironment, error) {
	var env models.ProjectEnvironment

	err := row.Scan(
		&env.ProjectID, &env.OrganizationID, &env.DockerfileText, &env.ValidationStatus,
		&env.ValidatedImageReference, &env.LastValidationLog, &env.ValidatedAt,
		&env.GenerationStatus, &env.GenerationLog, &env.UpdatedByUserID,
		&env.CreatedAt, &env.UpdatedAt,
	)
	if err != nil {
		return models.ProjectEnvironment{}, err
	}

	env.CreatedAt = env.CreatedAt.UTC()

	env.UpdatedAt = env.UpdatedAt.UTC()
	if env.ValidatedAt != nil {
		u := env.ValidatedAt.UTC()
		env.ValidatedAt = &u
	}

	return env, nil
}
