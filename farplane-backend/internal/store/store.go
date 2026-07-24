package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// ErrAlreadySetup is returned when first-time setup races or runs after an org exists.
var ErrAlreadySetup = errors.New("install already set up")

// ErrNotFound is returned when a row is missing.
var ErrNotFound = errors.New("not found")

// Store runs auth and setup queries against Postgres.
type Store struct {
	pool *pgxpool.Pool
}

// New builds a Store over an open pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// NeedsSetup reports whether organizations has zero rows.
func (s *Store) NeedsSetup(ctx context.Context) (bool, error) {
	var exists bool

	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations LIMIT 1)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("needs setup: %w", err)
	}

	return !exists, nil
}

// SetupPasswordInput is the first-owner password signup payload.
type SetupPasswordInput struct {
	OrganizationName string
	Email            string
	DisplayName      string
	PasswordHash     string
	SessionToken     string
	SessionExpiresAt time.Time
}

// SetupPasswordResult is the created first install.
type SetupPasswordResult struct {
	User         models.User
	Organization models.Organization
	Member       models.OrganizationMember
	Session      models.Session
}

// CompletePasswordSetup creates user, org, owner membership, and session in one transaction.
// It uses an advisory lock so concurrent setup attempts yield at most one success.
func (s *Store) CompletePasswordSetup(ctx context.Context, in SetupPasswordInput) (SetupPasswordResult, error) {
	var out SetupPasswordResult

	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSetup(ctx, tx); err != nil {
			return err
		}

		needs, err := needsSetupTx(ctx, tx)
		if err != nil {
			return err
		}

		if !needs {
			return ErrAlreadySetup
		}

		now := time.Now().UTC()

		user, err := insertUser(ctx, tx, in.Email, &in.PasswordHash, in.DisplayName, nil, now)
		if err != nil {
			return err
		}

		org, err := insertOrganization(ctx, tx, in.OrganizationName, now)
		if err != nil {
			return err
		}

		member, err := insertOrganizationMember(ctx, tx, org.ID, user.ID, models.OrganizationRoleOwner, now)
		if err != nil {
			return err
		}

		session, err := insertSession(ctx, tx, in.SessionToken, user.ID, in.SessionExpiresAt, now)
		if err != nil {
			return err
		}

		out = SetupPasswordResult{
			User:         user,
			Organization: org,
			Member:       member,
			Session:      session,
		}

		return nil
	})
	if err != nil {
		return SetupPasswordResult{}, err
	}

	return out, nil
}

// SetupGoogleInput is the first-owner Google signup payload.
type SetupGoogleInput struct {
	OrganizationName string
	Email            string
	DisplayName      string
	AvatarURL        *string
	ProviderSubject  string
	SessionToken     string
	SessionExpiresAt time.Time
}

// CompleteGoogleSetup creates user + Google identity, org, owner membership, and session.
func (s *Store) CompleteGoogleSetup(ctx context.Context, in SetupGoogleInput) (SetupPasswordResult, error) {
	var out SetupPasswordResult

	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSetup(ctx, tx); err != nil {
			return err
		}

		needs, err := needsSetupTx(ctx, tx)
		if err != nil {
			return err
		}

		if !needs {
			return ErrAlreadySetup
		}

		now := time.Now().UTC()

		user, err := insertUser(ctx, tx, in.Email, nil, in.DisplayName, in.AvatarURL, now)
		if err != nil {
			return err
		}

		if err := insertUserIdentity(ctx, tx, user.ID, models.IdentityProviderGoogle, in.ProviderSubject, now); err != nil {
			return err
		}

		org, err := insertOrganization(ctx, tx, in.OrganizationName, now)
		if err != nil {
			return err
		}

		member, err := insertOrganizationMember(ctx, tx, org.ID, user.ID, models.OrganizationRoleOwner, now)
		if err != nil {
			return err
		}

		session, err := insertSession(ctx, tx, in.SessionToken, user.ID, in.SessionExpiresAt, now)
		if err != nil {
			return err
		}

		out = SetupPasswordResult{
			User:         user,
			Organization: org,
			Member:       member,
			Session:      session,
		}

		return nil
	})
	if err != nil {
		return SetupPasswordResult{}, err
	}

	return out, nil
}

// UserWithOrg is the authenticated principal for /me and login responses.
type UserWithOrg struct {
	User         models.User
	Organization models.Organization
	Role         string
}

// GetUserByEmail loads a user by case-insensitive email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	const q = `
		SELECT id, email, password_hash, display_name, avatar_url, created_at, updated_at
		FROM users
		WHERE lower(email) = lower($1)
	`

	user, err := scanUser(s.pool.QueryRow(ctx, q, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, ErrNotFound
	}

	if err != nil {
		return models.User{}, fmt.Errorf("get user by email: %w", err)
	}

	return user, nil
}

// GetUserByID loads a user by id.
func (s *Store) GetUserByID(ctx context.Context, userID string) (models.User, error) {
	const q = `
		SELECT id, email, password_hash, display_name, avatar_url, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user, err := scanUser(s.pool.QueryRow(ctx, q, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, ErrNotFound
	}

	if err != nil {
		return models.User{}, fmt.Errorf("get user by id: %w", err)
	}

	return user, nil
}

// GetUserWithOrgByUserID returns the user and their first organization membership.
// One install = one organization; a user should have at most one membership for now.
func (s *Store) GetUserWithOrgByUserID(ctx context.Context, userID string) (UserWithOrg, error) {
	const q = `
		SELECT
			u.id, u.email, u.password_hash, u.display_name, u.avatar_url, u.created_at, u.updated_at,
			o.id, o.name, o.created_at, o.updated_at,
			m.role
		FROM users u
		JOIN organization_members m ON m.user_id = u.id
		JOIN organizations o ON o.id = m.organization_id
		WHERE u.id = $1
		ORDER BY m.created_at ASC
		LIMIT 1
	`

	var out UserWithOrg

	err := s.pool.QueryRow(ctx, q, userID).Scan(
		&out.User.ID, &out.User.Email, &out.User.PasswordHash, &out.User.DisplayName, &out.User.AvatarURL,
		&out.User.CreatedAt, &out.User.UpdatedAt,
		&out.Organization.ID, &out.Organization.Name, &out.Organization.CreatedAt, &out.Organization.UpdatedAt,
		&out.Role,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserWithOrg{}, ErrNotFound
	}

	if err != nil {
		return UserWithOrg{}, fmt.Errorf("get user with org: %w", err)
	}

	out.User.CreatedAt = out.User.CreatedAt.UTC()
	out.User.UpdatedAt = out.User.UpdatedAt.UTC()
	out.Organization.CreatedAt = out.Organization.CreatedAt.UTC()
	out.Organization.UpdatedAt = out.Organization.UpdatedAt.UTC()

	return out, nil
}

// GetUserIDByGoogleSubject finds a user linked to a Google subject.
func (s *Store) GetUserIDByGoogleSubject(ctx context.Context, subject string) (string, error) {
	const q = `
		SELECT user_id
		FROM user_identities
		WHERE provider = $1 AND provider_subject = $2
	`

	var userID string

	err := s.pool.QueryRow(ctx, q, models.IdentityProviderGoogle, subject).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}

	if err != nil {
		return "", fmt.Errorf("get user by google subject: %w", err)
	}

	return userID, nil
}

// CreateSession inserts a new session for the user.
// token is the raw cookie value; only its hash is stored.
func (s *Store) CreateSession(ctx context.Context, token, userID string, expiresAt time.Time) (models.Session, error) {
	now := time.Now().UTC()

	session, err := insertSession(ctx, s.pool, token, userID, expiresAt, now)
	if err != nil {
		return models.Session{}, err
	}

	return session, nil
}

// GetValidSessionUserID returns the user id for a non-expired session token.
// token is the raw cookie value.
func (s *Store) GetValidSessionUserID(ctx context.Context, token string, now time.Time) (string, error) {
	const q = `
		SELECT user_id
		FROM sessions
		WHERE token = $1 AND expires_at > $2
	`

	var userID string

	err := s.pool.QueryRow(ctx, q, auth.HashSessionToken(token), now.UTC()).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}

	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}

	return userID, nil
}

// DeleteSessionByToken removes a session (logout). Missing rows are ignored.
// token is the raw cookie value.
func (s *Store) DeleteSessionByToken(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, auth.HashSessionToken(token))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func lockSetup(ctx context.Context, tx pgx.Tx) error {
	// Stable key for first-time setup serialization across connections.
	const setupLockKey int64 = 0x466172706c616e65 // "Farplane" as hex-ish constant
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, setupLockKey); err != nil {
		return fmt.Errorf("setup lock: %w", err)
	}

	return nil
}

func needsSetupTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	var exists bool

	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations LIMIT 1)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("needs setup: %w", err)
	}

	return !exists, nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func insertUser(ctx context.Context, q querier, email string, passwordHash *string, displayName string, avatarURL *string, now time.Time) (models.User, error) {
	const sql = `
		INSERT INTO users (email, password_hash, display_name, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id, email, password_hash, display_name, avatar_url, created_at, updated_at
	`

	user, err := scanUser(q.QueryRow(ctx, sql, email, passwordHash, displayName, avatarURL, now))
	if err != nil {
		return models.User{}, fmt.Errorf("insert user: %w", err)
	}

	return user, nil
}

func insertUserIdentity(ctx context.Context, q querier, userID, provider, subject string, now time.Time) error {
	const sql = `
		INSERT INTO user_identities (user_id, provider, provider_subject, created_at)
		VALUES ($1, $2, $3, $4)
	`
	if _, err := q.Exec(ctx, sql, userID, provider, subject, now); err != nil {
		return fmt.Errorf("insert user identity: %w", err)
	}

	return nil
}

func insertOrganization(ctx context.Context, q querier, name string, now time.Time) (models.Organization, error) {
	const sql = `
		INSERT INTO organizations (name, created_at, updated_at)
		VALUES ($1, $2, $2)
		RETURNING id, name, created_at, updated_at
	`

	var org models.Organization

	err := q.QueryRow(ctx, sql, name, now).Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return models.Organization{}, fmt.Errorf("insert organization: %w", err)
	}

	org.CreatedAt = org.CreatedAt.UTC()
	org.UpdatedAt = org.UpdatedAt.UTC()

	return org, nil
}

func insertOrganizationMember(ctx context.Context, q querier, organizationID, userID, role string, now time.Time) (models.OrganizationMember, error) {
	const sql = `
		INSERT INTO organization_members (organization_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, organization_id, user_id, role, created_at
	`

	var m models.OrganizationMember

	err := q.QueryRow(ctx, sql, organizationID, userID, role, now).Scan(
		&m.ID, &m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt,
	)
	if err != nil {
		return models.OrganizationMember{}, fmt.Errorf("insert organization member: %w", err)
	}

	m.CreatedAt = m.CreatedAt.UTC()

	return m, nil
}

func insertSession(ctx context.Context, q querier, rawToken, userID string, expiresAt, now time.Time) (models.Session, error) {
	const sql = `
		INSERT INTO sessions (token, user_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, token, user_id, expires_at, created_at
	`

	tokenHash := auth.HashSessionToken(rawToken)

	var sess models.Session

	err := q.QueryRow(ctx, sql, tokenHash, userID, expiresAt.UTC(), now).Scan(
		&sess.ID, &sess.Token, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt,
	)
	if err != nil {
		return models.Session{}, fmt.Errorf("insert session: %w", err)
	}

	sess.ExpiresAt = sess.ExpiresAt.UTC()
	sess.CreatedAt = sess.CreatedAt.UTC()

	return sess, nil
}

func scanUser(row pgx.Row) (models.User, error) {
	var u models.User

	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return models.User{}, err
	}

	u.CreatedAt = u.CreatedAt.UTC()
	u.UpdatedAt = u.UpdatedAt.UTC()

	return u, nil
}
