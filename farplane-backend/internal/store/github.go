package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// ErrGitHubInstallationOwned is returned when an installation belongs to another organization.
var ErrGitHubInstallationOwned = errors.New("github installation owned by another organization")

// UpsertGitHubInstallationInput creates or reactivates an installation row.
type UpsertGitHubInstallationInput struct {
	OrganizationID       string
	GitHubInstallationID int64
	GitHubAccountID      int64
	GitHubAccountLogin   string
	GitHubAccountType    string
	RepositorySelection  string
	ConnectedByUserID    string
	SuspendedAt          *time.Time
}

// UpsertGitHubInstallation inserts or updates by github_installation_id.
// On update for the same organization, connected_by_user_id is preserved so a
// member cannot steal disconnect rights. Cross-organization reassignment is rejected.
func (s *Store) UpsertGitHubInstallation(ctx context.Context, in UpsertGitHubInstallationInput) (models.GitHubInstallation, error) {
	now := time.Now().UTC()
	var out models.GitHubInstallation
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		existing, err := getGitHubInstallationByGitHubIDTx(ctx, tx, in.GitHubInstallationID)
		if err == nil {
			if existing.OrganizationID != in.OrganizationID && existing.UninstalledAt == nil {
				return ErrGitHubInstallationOwned
			}
			const update = `
				UPDATE github_installations SET
					github_account_id = $2,
					github_account_login = $3,
					github_account_type = $4,
					repository_selection = $5,
					suspended_at = $6,
					uninstalled_at = NULL,
					updated_at = $7,
					organization_id = $8,
					connected_by_user_id = CASE
						WHEN uninstalled_at IS NOT NULL THEN $9
						ELSE connected_by_user_id
					END
				WHERE github_installation_id = $1
				RETURNING id, organization_id, github_installation_id, github_account_id, github_account_login,
					github_account_type, repository_selection, connected_by_user_id, suspended_at, uninstalled_at,
					created_at, updated_at
			`
			inst, err := scanGitHubInstallation(tx.QueryRow(
				ctx, update,
				in.GitHubInstallationID, in.GitHubAccountID, in.GitHubAccountLogin, in.GitHubAccountType,
				in.RepositorySelection, in.SuspendedAt, now, in.OrganizationID, in.ConnectedByUserID,
			))
			if err != nil {
				return fmt.Errorf("update github installation: %w", err)
			}
			out = inst
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		const insert = `
			INSERT INTO github_installations (
				organization_id, github_installation_id, github_account_id, github_account_login,
				github_account_type, repository_selection, connected_by_user_id, suspended_at,
				uninstalled_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, $9, $9)
			RETURNING id, organization_id, github_installation_id, github_account_id, github_account_login,
				github_account_type, repository_selection, connected_by_user_id, suspended_at, uninstalled_at,
				created_at, updated_at
		`
		inst, err := scanGitHubInstallation(tx.QueryRow(
			ctx, insert,
			in.OrganizationID, in.GitHubInstallationID, in.GitHubAccountID, in.GitHubAccountLogin,
			in.GitHubAccountType, in.RepositorySelection, in.ConnectedByUserID, in.SuspendedAt, now,
		))
		if err != nil {
			return fmt.Errorf("insert github installation: %w", err)
		}
		out = inst
		return nil
	})
	if err != nil {
		return models.GitHubInstallation{}, err
	}
	return out, nil
}

func getGitHubInstallationByGitHubIDTx(ctx context.Context, tx pgx.Tx, githubInstallationID int64) (models.GitHubInstallation, error) {
	const q = `
		SELECT id, organization_id, github_installation_id, github_account_id, github_account_login,
			github_account_type, repository_selection, connected_by_user_id, suspended_at, uninstalled_at,
			created_at, updated_at
		FROM github_installations
		WHERE github_installation_id = $1
	`
	inst, err := scanGitHubInstallation(tx.QueryRow(ctx, q, githubInstallationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.GitHubInstallation{}, ErrNotFound
	}
	if err != nil {
		return models.GitHubInstallation{}, fmt.Errorf("get github installation by github id: %w", err)
	}
	return inst, nil
}

// GitHubRepoSync is one repository row to upsert into the cache.
type GitHubRepoSync struct {
	GitHubRepositoryID int64
	FullName           string
	DefaultBranch      string
	Private            bool
	HTMLURL            string
}

// ReplaceGitHubRepositories replaces the active repo set for an installation.
// Repos not in the list are soft-removed; matching projects are marked revoked.
func (s *Store) ReplaceGitHubRepositories(ctx context.Context, installationRowID string, repos []GitHubRepoSync) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		now := time.Now().UTC()
		keep := make([]int64, 0, len(repos))
		for _, repo := range repos {
			keep = append(keep, repo.GitHubRepositoryID)
			const upsert = `
				INSERT INTO github_repositories (
					github_installation_id, github_repository_id, full_name, default_branch,
					private, html_url, removed_at, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, $7)
				ON CONFLICT (github_installation_id, github_repository_id) DO UPDATE SET
					full_name = EXCLUDED.full_name,
					default_branch = EXCLUDED.default_branch,
					private = EXCLUDED.private,
					html_url = EXCLUDED.html_url,
					removed_at = NULL,
					updated_at = EXCLUDED.updated_at
			`
			branch := repo.DefaultBranch
			if branch == "" {
				branch = "main"
			}
			if _, err := tx.Exec(ctx, upsert,
				installationRowID, repo.GitHubRepositoryID, repo.FullName, branch,
				repo.Private, repo.HTMLURL, now,
			); err != nil {
				return fmt.Errorf("upsert github repository: %w", err)
			}
		}

		var removeQ string
		var args []any
		if len(keep) == 0 {
			removeQ = `
				UPDATE github_repositories
				SET removed_at = $2, updated_at = $2
				WHERE github_installation_id = $1 AND removed_at IS NULL
				RETURNING github_repository_id
			`
			args = []any{installationRowID, now}
		} else {
			removeQ = `
				UPDATE github_repositories
				SET removed_at = $3, updated_at = $3
				WHERE github_installation_id = $1
					AND removed_at IS NULL
					AND NOT (github_repository_id = ANY ($2::bigint[]))
				RETURNING github_repository_id
			`
			args = []any{installationRowID, keep, now}
		}

		rows, err := tx.Query(ctx, removeQ, args...)
		if err != nil {
			return fmt.Errorf("soft-remove github repositories: %w", err)
		}
		defer rows.Close()
		var removedIDs []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("scan removed repository: %w", err)
			}
			removedIDs = append(removedIDs, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(removedIDs) > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE projects
				SET github_access_status = $2, updated_at = $3
				WHERE github_installation_id = $1
					AND github_repository_id = ANY ($4::bigint[])
					AND github_access_status = $5
			`, installationRowID, models.ProjectGitHubAccessRevoked, now, removedIDs, models.ProjectGitHubAccessActive); err != nil {
				return fmt.Errorf("revoke projects for removed repos: %w", err)
			}
		}

		// Restore active status for repos that came back.
		if len(keep) > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE projects
				SET github_access_status = $2, updated_at = $3
				WHERE github_installation_id = $1
					AND github_repository_id = ANY ($4::bigint[])
					AND github_access_status = $5
			`, installationRowID, models.ProjectGitHubAccessActive, now, keep, models.ProjectGitHubAccessRevoked); err != nil {
				return fmt.Errorf("reactivate projects: %w", err)
			}
		}
		return nil
	})
}

// SoftRemoveGitHubRepositories marks specific repos removed and revokes projects.
func (s *Store) SoftRemoveGitHubRepositories(ctx context.Context, installationRowID string, githubRepositoryIDs []int64) error {
	if len(githubRepositoryIDs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE github_repositories
			SET removed_at = $3, updated_at = $3
			WHERE github_installation_id = $1
				AND github_repository_id = ANY ($2::bigint[])
				AND removed_at IS NULL
		`, installationRowID, githubRepositoryIDs, now); err != nil {
			return fmt.Errorf("soft-remove github repositories: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE projects
			SET github_access_status = $2, updated_at = $3
			WHERE github_installation_id = $1
				AND github_repository_id = ANY ($4::bigint[])
		`, installationRowID, models.ProjectGitHubAccessRevoked, now, githubRepositoryIDs); err != nil {
			return fmt.Errorf("revoke projects: %w", err)
		}
		return nil
	})
}

// MarkGitHubInstallationUninstalled soft-uninstalls and revokes all its projects.
func (s *Store) MarkGitHubInstallationUninstalled(ctx context.Context, githubInstallationID int64) error {
	now := time.Now().UTC()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var rowID string
		err := tx.QueryRow(ctx, `
			UPDATE github_installations
			SET uninstalled_at = $2, updated_at = $2
			WHERE github_installation_id = $1 AND uninstalled_at IS NULL
			RETURNING id
		`, githubInstallationID, now).Scan(&rowID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("mark installation uninstalled: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE github_repositories
			SET removed_at = $2, updated_at = $2
			WHERE github_installation_id = $1 AND removed_at IS NULL
		`, rowID, now); err != nil {
			return fmt.Errorf("soft-remove repos on uninstall: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE projects
			SET github_access_status = $2, updated_at = $3
			WHERE github_installation_id = $1
		`, rowID, models.ProjectGitHubAccessRevoked, now); err != nil {
			return fmt.Errorf("revoke projects on uninstall: %w", err)
		}
		return nil
	})
}

// SetGitHubInstallationSuspended updates suspended_at from webhooks.
// When suspending, projects on that install are marked revoked.
func (s *Store) SetGitHubInstallationSuspended(ctx context.Context, githubInstallationID int64, suspendedAt *time.Time) error {
	now := time.Now().UTC()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var rowID string
		err := tx.QueryRow(ctx, `
			UPDATE github_installations
			SET suspended_at = $2, updated_at = $3
			WHERE github_installation_id = $1 AND uninstalled_at IS NULL
			RETURNING id
		`, githubInstallationID, suspendedAt, now).Scan(&rowID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("set installation suspended: %w", err)
		}
		if suspendedAt != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE projects
				SET github_access_status = $2, updated_at = $3
				WHERE github_installation_id = $1
			`, rowID, models.ProjectGitHubAccessRevoked, now); err != nil {
				return fmt.Errorf("revoke projects on suspend: %w", err)
			}
		}
		return nil
	})
}

// ListGitHubInstallations returns active installations for an organization.
func (s *Store) ListGitHubInstallations(ctx context.Context, organizationID string) ([]models.GitHubInstallation, error) {
	const q = `
		SELECT id, organization_id, github_installation_id, github_account_id, github_account_login,
			github_account_type, repository_selection, connected_by_user_id, suspended_at, uninstalled_at,
			created_at, updated_at
		FROM github_installations
		WHERE organization_id = $1 AND uninstalled_at IS NULL
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list github installations: %w", err)
	}
	defer rows.Close()
	var out []models.GitHubInstallation
	for rows.Next() {
		inst, err := scanGitHubInstallation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

// GetGitHubInstallationByID loads one installation row by Farplane id.
func (s *Store) GetGitHubInstallationByID(ctx context.Context, id string) (models.GitHubInstallation, error) {
	const q = `
		SELECT id, organization_id, github_installation_id, github_account_id, github_account_login,
			github_account_type, repository_selection, connected_by_user_id, suspended_at, uninstalled_at,
			created_at, updated_at
		FROM github_installations
		WHERE id = $1
	`
	inst, err := scanGitHubInstallation(s.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.GitHubInstallation{}, ErrNotFound
	}
	if err != nil {
		return models.GitHubInstallation{}, fmt.Errorf("get github installation: %w", err)
	}
	return inst, nil
}

// GetGitHubInstallationByGitHubID loads by GitHub's installation id.
func (s *Store) GetGitHubInstallationByGitHubID(ctx context.Context, githubInstallationID int64) (models.GitHubInstallation, error) {
	const q = `
		SELECT id, organization_id, github_installation_id, github_account_id, github_account_login,
			github_account_type, repository_selection, connected_by_user_id, suspended_at, uninstalled_at,
			created_at, updated_at
		FROM github_installations
		WHERE github_installation_id = $1
	`
	inst, err := scanGitHubInstallation(s.pool.QueryRow(ctx, q, githubInstallationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.GitHubInstallation{}, ErrNotFound
	}
	if err != nil {
		return models.GitHubInstallation{}, fmt.Errorf("get github installation by github id: %w", err)
	}
	return inst, nil
}

// PickableGitHubRepository is a unioned repo for the Project picker.
type PickableGitHubRepository struct {
	GitHubRepositoryID   int64
	FullName             string
	DefaultBranch        string
	Private              bool
	HTMLURL              string
	GitHubInstallationID string // Farplane row id
	GitHubAccountType    string
	GitHubAccountLogin   string
}

// ListPickableGitHubRepositories returns active repos for an org.
// When the same github_repository_id appears twice, Organization installs win.
func (s *Store) ListPickableGitHubRepositories(ctx context.Context, organizationID string) ([]PickableGitHubRepository, error) {
	const q = `
		SELECT DISTINCT ON (r.github_repository_id)
			r.github_repository_id, r.full_name, r.default_branch, r.private, r.html_url,
			i.id, i.github_account_type, i.github_account_login
		FROM github_repositories r
		JOIN github_installations i ON i.id = r.github_installation_id
		WHERE i.organization_id = $1
			AND i.uninstalled_at IS NULL
			AND i.suspended_at IS NULL
			AND r.removed_at IS NULL
		ORDER BY r.github_repository_id,
			CASE WHEN i.github_account_type = 'Organization' THEN 0 ELSE 1 END,
			i.created_at ASC
	`
	rows, err := s.pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list pickable github repositories: %w", err)
	}
	defer rows.Close()
	var out []PickableGitHubRepository
	for rows.Next() {
		var repo PickableGitHubRepository
		if err := rows.Scan(
			&repo.GitHubRepositoryID, &repo.FullName, &repo.DefaultBranch, &repo.Private, &repo.HTMLURL,
			&repo.GitHubInstallationID, &repo.GitHubAccountType, &repo.GitHubAccountLogin,
		); err != nil {
			return nil, fmt.Errorf("scan pickable repository: %w", err)
		}
		out = append(out, repo)
	}
	return out, rows.Err()
}

// GetPickableGitHubRepository loads one active pickable repo for an org.
func (s *Store) GetPickableGitHubRepository(ctx context.Context, organizationID string, githubRepositoryID int64) (PickableGitHubRepository, error) {
	const q = `
		SELECT
			r.github_repository_id, r.full_name, r.default_branch, r.private, r.html_url,
			i.id, i.github_account_type, i.github_account_login
		FROM github_repositories r
		JOIN github_installations i ON i.id = r.github_installation_id
		WHERE i.organization_id = $1
			AND r.github_repository_id = $2
			AND i.uninstalled_at IS NULL
			AND i.suspended_at IS NULL
			AND r.removed_at IS NULL
		ORDER BY
			CASE WHEN i.github_account_type = 'Organization' THEN 0 ELSE 1 END,
			i.created_at ASC
		LIMIT 1
	`
	var repo PickableGitHubRepository
	err := s.pool.QueryRow(ctx, q, organizationID, githubRepositoryID).Scan(
		&repo.GitHubRepositoryID, &repo.FullName, &repo.DefaultBranch, &repo.Private, &repo.HTMLURL,
		&repo.GitHubInstallationID, &repo.GitHubAccountType, &repo.GitHubAccountLogin,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PickableGitHubRepository{}, ErrNotFound
	}
	if err != nil {
		return PickableGitHubRepository{}, fmt.Errorf("get pickable github repository: %w", err)
	}
	return repo, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanGitHubInstallation(row scannable) (models.GitHubInstallation, error) {
	var inst models.GitHubInstallation
	err := row.Scan(
		&inst.ID, &inst.OrganizationID, &inst.GitHubInstallationID, &inst.GitHubAccountID,
		&inst.GitHubAccountLogin, &inst.GitHubAccountType, &inst.RepositorySelection,
		&inst.ConnectedByUserID, &inst.SuspendedAt, &inst.UninstalledAt, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return models.GitHubInstallation{}, err
	}
	inst.CreatedAt = inst.CreatedAt.UTC()
	inst.UpdatedAt = inst.UpdatedAt.UTC()
	if inst.SuspendedAt != nil {
		t := inst.SuspendedAt.UTC()
		inst.SuspendedAt = &t
	}
	if inst.UninstalledAt != nil {
		t := inst.UninstalledAt.UTC()
		inst.UninstalledAt = &t
	}
	return inst, nil
}
