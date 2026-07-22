package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/secretbox"
)

// ErrGitHubAppCredentialsExist is returned when this install already has App credentials.
var ErrGitHubAppCredentialsExist = errors.New("github app credentials already exist")

// InsertGitHubAppCredentialsInput stores this install's GitHub App credentials once.
type InsertGitHubAppCredentialsInput struct {
	GitHubAppID     int64
	GitHubAppSlug   string
	PrivateKeyPEM   string
	WebhookSecret   string
	ClientID        string
	ClientSecret    string
	CreatedByUserID string
	EncryptionKey   string // SESSION_SECRET
}

// InsertGitHubAppCredentials stores credentials only when none exist yet.
func (s *Store) InsertGitHubAppCredentials(ctx context.Context, in InsertGitHubAppCredentialsInput) (models.GitHubAppCredentials, error) {
	now := time.Now().UTC()
	pemEnc, err := secretbox.Encrypt(in.EncryptionKey, []byte(in.PrivateKeyPEM))
	if err != nil {
		return models.GitHubAppCredentials{}, fmt.Errorf("encrypt private key: %w", err)
	}
	webhookEnc, err := secretbox.Encrypt(in.EncryptionKey, []byte(in.WebhookSecret))
	if err != nil {
		return models.GitHubAppCredentials{}, fmt.Errorf("encrypt webhook secret: %w", err)
	}
	var clientIDEnc, clientSecretEnc []byte
	if in.ClientID != "" {
		clientIDEnc, err = secretbox.Encrypt(in.EncryptionKey, []byte(in.ClientID))
		if err != nil {
			return models.GitHubAppCredentials{}, fmt.Errorf("encrypt client id: %w", err)
		}
	}
	if in.ClientSecret != "" {
		clientSecretEnc, err = secretbox.Encrypt(in.EncryptionKey, []byte(in.ClientSecret))
		if err != nil {
			return models.GitHubAppCredentials{}, fmt.Errorf("encrypt client secret: %w", err)
		}
	}

	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM github_app_credentials LIMIT 1)`).Scan(&exists); err != nil {
			return fmt.Errorf("check github app credentials: %w", err)
		}
		if exists {
			return ErrGitHubAppCredentialsExist
		}
		const q = `
			INSERT INTO github_app_credentials (
				github_app_id, github_app_slug, private_key_pem_encrypted, webhook_secret_encrypted,
				client_id_encrypted, client_secret_encrypted, created_by_user_id, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		`
		_, err := tx.Exec(ctx, q,
			in.GitHubAppID, in.GitHubAppSlug, pemEnc, webhookEnc,
			clientIDEnc, clientSecretEnc, in.CreatedByUserID, now,
		)
		return err
	})
	if err != nil {
		return models.GitHubAppCredentials{}, err
	}
	return s.GetGitHubAppCredentials(ctx, in.EncryptionKey)
}

// GetGitHubAppCredentials loads and decrypts the singleton GitHub App credentials.
func (s *Store) GetGitHubAppCredentials(ctx context.Context, encryptionKey string) (models.GitHubAppCredentials, error) {
	const q = `
		SELECT id, github_app_id, github_app_slug, private_key_pem_encrypted, webhook_secret_encrypted,
			client_id_encrypted, client_secret_encrypted, created_by_user_id, created_at, updated_at
		FROM github_app_credentials
		LIMIT 1
	`
	var (
		out             models.GitHubAppCredentials
		pemEnc          []byte
		webhookEnc      []byte
		clientIDEnc     []byte
		clientSecretEnc []byte
	)
	err := s.pool.QueryRow(ctx, q).Scan(
		&out.ID, &out.GitHubAppID, &out.GitHubAppSlug, &pemEnc, &webhookEnc,
		&clientIDEnc, &clientSecretEnc, &out.CreatedByUserID, &out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.GitHubAppCredentials{}, ErrNotFound
	}
	if err != nil {
		return models.GitHubAppCredentials{}, fmt.Errorf("get github app credentials: %w", err)
	}

	pem, err := secretbox.Decrypt(encryptionKey, pemEnc)
	if err != nil {
		return models.GitHubAppCredentials{}, fmt.Errorf("decrypt private key: %w", err)
	}
	webhook, err := secretbox.Decrypt(encryptionKey, webhookEnc)
	if err != nil {
		return models.GitHubAppCredentials{}, fmt.Errorf("decrypt webhook secret: %w", err)
	}
	out.PrivateKeyPEM = string(pem)
	out.WebhookSecret = string(webhook)
	if len(clientIDEnc) > 0 {
		raw, err := secretbox.Decrypt(encryptionKey, clientIDEnc)
		if err != nil {
			return models.GitHubAppCredentials{}, fmt.Errorf("decrypt client id: %w", err)
		}
		out.ClientID = string(raw)
	}
	if len(clientSecretEnc) > 0 {
		raw, err := secretbox.Decrypt(encryptionKey, clientSecretEnc)
		if err != nil {
			return models.GitHubAppCredentials{}, fmt.Errorf("decrypt client secret: %w", err)
		}
		out.ClientSecret = string(raw)
	}
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}

// HasGitHubAppCredentials reports whether credentials exist (without decrypting).
func (s *Store) HasGitHubAppCredentials(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM github_app_credentials LIMIT 1)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has github app credentials: %w", err)
	}
	return exists, nil
}
