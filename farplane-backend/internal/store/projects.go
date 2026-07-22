package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// CreateProjectInput binds a Farplane Project to a GitHub repository.
type CreateProjectInput struct {
	OrganizationID       string
	Name                 string
	GitHubRepositoryID   int64
	GitHubInstallationID string
	DefaultBranch        string
	GitHubFullName       string
	CreatedByUserID      string
}

// CreateProject inserts a Project. Duplicate org+repo returns a conflict-style error.
var ErrProjectRepoExists = errors.New("project already exists for repository")

// CreateProject creates a Project from a pickable GitHub repository.
func (s *Store) CreateProject(ctx context.Context, in CreateProjectInput) (models.Project, error) {
	now := time.Now().UTC()
	branch := in.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	const q = `
		INSERT INTO projects (
			organization_id, name, github_repository_id, github_installation_id,
			default_branch, github_full_name, github_access_status, created_by_user_id,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		RETURNING id, organization_id, name, github_repository_id, github_installation_id,
			default_branch, github_full_name, github_access_status, created_by_user_id,
			created_at, updated_at
	`
	project, err := scanProject(s.pool.QueryRow(
		ctx, q,
		in.OrganizationID, in.Name, in.GitHubRepositoryID, in.GitHubInstallationID,
		branch, in.GitHubFullName, models.ProjectGitHubAccessActive, in.CreatedByUserID, now,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return models.Project{}, ErrProjectRepoExists
		}
		return models.Project{}, fmt.Errorf("create project: %w", err)
	}
	return project, nil
}

// ListProjects returns projects for an organization.
func (s *Store) ListProjects(ctx context.Context, organizationID string) ([]models.Project, error) {
	const q = `
		SELECT id, organization_id, name, github_repository_id, github_installation_id,
			default_branch, github_full_name, github_access_status, created_by_user_id,
			created_at, updated_at
		FROM projects
		WHERE organization_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []models.Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, project)
	}
	return out, rows.Err()
}

// GetProject loads a project by id.
func (s *Store) GetProject(ctx context.Context, id string) (models.Project, error) {
	const q = `
		SELECT id, organization_id, name, github_repository_id, github_installation_id,
			default_branch, github_full_name, github_access_status, created_by_user_id,
			created_at, updated_at
		FROM projects
		WHERE id = $1
	`
	project, err := scanProject(s.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Project{}, ErrNotFound
	}
	if err != nil {
		return models.Project{}, fmt.Errorf("get project: %w", err)
	}
	return project, nil
}

func scanProject(row scannable) (models.Project, error) {
	var p models.Project
	err := row.Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.GitHubRepositoryID, &p.GitHubInstallationID,
		&p.DefaultBranch, &p.GitHubFullName, &p.GitHubAccessStatus, &p.CreatedByUserID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return models.Project{}, err
	}
	p.CreatedAt = p.CreatedAt.UTC()
	p.UpdatedAt = p.UpdatedAt.UTC()
	return p, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
