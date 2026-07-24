package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/secretbox"
)

// ListOrganizationSecretMeta returns well-known secrets with is_set only (never values).
func (s *Store) ListOrganizationSecretMeta(ctx context.Context, organizationID string) ([]models.OrganizationSecretMeta, error) {
	const q = `
		SELECT name, updated_at
		FROM organization_secrets
		WHERE organization_id = $1
	`

	rows, err := s.pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list organization secrets: %w", err)
	}
	defer rows.Close()

	setAt := map[string]time.Time{}

	for rows.Next() {
		var (
			name      string
			updatedAt time.Time
		)
		if err := rows.Scan(&name, &updatedAt); err != nil {
			return nil, err
		}

		setAt[name] = updatedAt.UTC()
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.OrganizationSecretMeta, 0, len(agents.WellKnownSecretNames))
	for _, name := range agents.WellKnownSecretNames {
		meta := models.OrganizationSecretMeta{Name: name, IsSet: false}
		if t, ok := setAt[name]; ok {
			meta.IsSet = true
			meta.UpdatedAt = &t
		}

		out = append(out, meta)
	}

	return out, nil
}

// SetOrganizationSecret encrypts and upserts a secret value.
func (s *Store) SetOrganizationSecret(ctx context.Context, organizationID, name, value, encryptionKey, userID string) error {
	if value == "" {
		return errors.New("secret value is empty")
	}

	enc, err := secretbox.Encrypt(encryptionKey, []byte(value))
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	var createdBy, updatedBy *string
	if userID != "" {
		createdBy = &userID
		updatedBy = &userID
	}

	const q = `
		INSERT INTO organization_secrets (
			organization_id, name, value_encrypted, created_by_user_id, updated_by_user_id,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (organization_id, name) DO UPDATE SET
			value_encrypted = EXCLUDED.value_encrypted,
			updated_by_user_id = EXCLUDED.updated_by_user_id,
			updated_at = EXCLUDED.updated_at
	`

	_, err = s.pool.Exec(ctx, q, organizationID, name, enc, createdBy, updatedBy, now)
	if err != nil {
		return fmt.Errorf("set organization secret: %w", err)
	}

	return nil
}

// ClearOrganizationSecret deletes a secret by name.
func (s *Store) ClearOrganizationSecret(ctx context.Context, organizationID, name string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM organization_secrets
		WHERE organization_id = $1 AND name = $2
	`, organizationID, name)
	if err != nil {
		return fmt.Errorf("clear organization secret: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// OrganizationSecretIsSet reports whether a named secret exists.
func (s *Store) OrganizationSecretIsSet(ctx context.Context, organizationID, name string) (bool, error) {
	var exists bool

	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_secrets
			WHERE organization_id = $1 AND name = $2
		)
	`, organizationID, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("organization secret is set: %w", err)
	}

	return exists, nil
}

// DecryptOrganizationSecrets returns all org secrets decrypted (for InjectSecrets).
func (s *Store) DecryptOrganizationSecrets(ctx context.Context, organizationID, encryptionKey string) (map[string]string, error) {
	const q = `
		SELECT name, value_encrypted
		FROM organization_secrets
		WHERE organization_id = $1
	`

	rows, err := s.pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list encrypted secrets: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}

	for rows.Next() {
		var (
			name string
			enc  []byte
		)
		if err := rows.Scan(&name, &enc); err != nil {
			return nil, err
		}

		plain, err := secretbox.Decrypt(encryptionKey, enc)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret %s: %w", name, err)
		}

		out[name] = string(plain)
	}

	return out, rows.Err()
}

// SetSecretsMap returns name->true for set secrets (agent availability).
func (s *Store) SetSecretsMap(ctx context.Context, organizationID string) (map[string]bool, error) {
	metas, err := s.ListOrganizationSecretMeta(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	out := map[string]bool{}

	for _, m := range metas {
		if m.IsSet {
			out[m.Name] = true
		}
	}

	return out, nil
}
