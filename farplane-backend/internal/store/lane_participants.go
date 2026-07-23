package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

const laneInviteTTL = 7 * 24 * time.Hour

// ListLaneParticipants returns seats for a lane.
func (s *Store) ListLaneParticipants(ctx context.Context, laneID string) ([]models.LaneParticipant, error) {
	const q = `
		SELECT id, lane_id, user_id, role, joined_at
		FROM lane_participants
		WHERE lane_id = $1
		ORDER BY joined_at ASC
	`
	rows, err := s.pool.Query(ctx, q, laneID)
	if err != nil {
		return nil, fmt.Errorf("list lane participants: %w", err)
	}
	defer rows.Close()
	var out []models.LaneParticipant
	for rows.Next() {
		p, err := scanLaneParticipant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListOrganizationMembersForInvite returns org members for Lane add-participant autocomplete.
func (s *Store) ListOrganizationMembersForInvite(ctx context.Context, organizationID string) ([]models.User, error) {
	const q = `
		SELECT u.id, u.email, u.password_hash, u.display_name, u.avatar_url, u.created_at, u.updated_at
		FROM users u
		JOIN organization_members m ON m.user_id = u.id
		WHERE m.organization_id = $1
		ORDER BY lower(u.display_name), lower(u.email)
	`
	rows, err := s.pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list org members: %w", err)
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// OrganizationMemberExists reports whether userID is a member of the organization.
func (s *Store) OrganizationMemberExists(ctx context.Context, organizationID, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_members
			WHERE organization_id = $1 AND user_id = $2
		)
	`, organizationID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("organization member exists: %w", err)
	}
	return exists, nil
}

// AddLaneParticipant seats an organization member on a lane immediately.
func (s *Store) AddLaneParticipant(ctx context.Context, laneID, userID string) (models.LaneParticipant, error) {
	now := time.Now().UTC()
	const q = `
		INSERT INTO lane_participants (lane_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (lane_id, user_id) DO UPDATE SET
			joined_at = lane_participants.joined_at
		RETURNING id, lane_id, user_id, role, joined_at
	`
	p, err := scanLaneParticipant(s.pool.QueryRow(
		ctx, q, laneID, userID, models.LaneParticipantRoleParticipant, now,
	))
	if err != nil {
		return models.LaneParticipant{}, fmt.Errorf("add lane participant: %w", err)
	}
	return p, nil
}

// RemoveLaneParticipant hard-deletes a non-owner seat.
func (s *Store) RemoveLaneParticipant(ctx context.Context, laneID, targetUserID string) error {
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM lane_participants
		WHERE lane_id = $1 AND user_id = $2
	`, laneID, targetUserID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == models.LaneParticipantRoleOwner {
		return ErrConflict
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM lane_participants
		WHERE lane_id = $1 AND user_id = $2
	`, laneID, targetUserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LeaveLane hard-deletes the caller's non-owner seat.
func (s *Store) LeaveLane(ctx context.Context, laneID, userID string) error {
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM lane_participants
		WHERE lane_id = $1 AND user_id = $2
	`, laneID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == models.LaneParticipantRoleOwner {
		return ErrConflict
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM lane_participants
		WHERE lane_id = $1 AND user_id = $2
	`, laneID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateLaneInviteInput creates an open multi-use invite.
type CreateLaneInviteInput struct {
	LaneID          string
	InvitedByUserID string
	ExpiresAt       *time.Time
}

// GetActiveLaneInvite returns the non-revoked, non-expired invite for a lane.
func (s *Store) GetActiveLaneInvite(ctx context.Context, laneID string) (models.LaneInvite, error) {
	now := time.Now().UTC()
	const q = `
		SELECT id, lane_id, token, invited_by_user_id, expires_at, revoked_at, created_at
		FROM lane_invites
		WHERE lane_id = $1
			AND revoked_at IS NULL
			AND (expires_at IS NULL OR expires_at > $2)
		ORDER BY created_at DESC
		LIMIT 1
	`
	inv, err := scanLaneInvite(s.pool.QueryRow(ctx, q, laneID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.LaneInvite{}, ErrNotFound
	}
	if err != nil {
		return models.LaneInvite{}, fmt.Errorf("get active lane invite: %w", err)
	}
	return inv, nil
}

// EnsureLaneInvite returns the active invite or mints a new one (7-day expiry).
func (s *Store) EnsureLaneInvite(ctx context.Context, in CreateLaneInviteInput) (models.LaneInvite, error) {
	if inv, err := s.GetActiveLaneInvite(ctx, in.LaneID); err == nil {
		return inv, nil
	} else if !errors.Is(err, ErrNotFound) {
		return models.LaneInvite{}, err
	}
	// Expired but non-revoked rows still hold the unique index; clear them first.
	if err := s.RevokeActiveLaneInvite(ctx, in.LaneID); err != nil && !errors.Is(err, ErrNotFound) {
		return models.LaneInvite{}, err
	}
	expires := time.Now().UTC().Add(laneInviteTTL)
	if in.ExpiresAt != nil {
		expires = in.ExpiresAt.UTC()
	}
	return s.insertLaneInvite(ctx, in.LaneID, in.InvitedByUserID, expires)
}

// RegenerateLaneInvite revokes the current non-revoked invite and mints a new token.
func (s *Store) RegenerateLaneInvite(ctx context.Context, laneID, invitedByUserID string) (models.LaneInvite, error) {
	now := time.Now().UTC()
	var out models.LaneInvite
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE lane_invites
			SET revoked_at = $2
			WHERE lane_id = $1 AND revoked_at IS NULL
		`, laneID, now); err != nil {
			return err
		}
		token, err := randomToken(32)
		if err != nil {
			return err
		}
		expires := now.Add(laneInviteTTL)
		var invitedBy *string
		if invitedByUserID != "" {
			invitedBy = &invitedByUserID
		}
		inv, err := scanLaneInvite(tx.QueryRow(ctx, `
			INSERT INTO lane_invites (
				lane_id, token, invited_by_user_id, expires_at, created_at
			) VALUES ($1,$2,$3,$4,$5)
			RETURNING id, lane_id, token, invited_by_user_id, expires_at, revoked_at, created_at
		`, laneID, token, invitedBy, expires, now))
		if err != nil {
			return fmt.Errorf("insert regenerated lane invite: %w", err)
		}
		out = inv
		return nil
	})
	if err != nil {
		return models.LaneInvite{}, err
	}
	return out, nil
}

// RevokeActiveLaneInvite revokes the current non-revoked invite for a lane.
func (s *Store) RevokeActiveLaneInvite(ctx context.Context, laneID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE lane_invites
		SET revoked_at = $2
		WHERE lane_id = $1 AND revoked_at IS NULL
	`, laneID, time.Now().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) insertLaneInvite(ctx context.Context, laneID, invitedByUserID string, expires time.Time) (models.LaneInvite, error) {
	token, err := randomToken(32)
	if err != nil {
		return models.LaneInvite{}, err
	}
	now := time.Now().UTC()
	var invitedBy *string
	if invitedByUserID != "" {
		invitedBy = &invitedByUserID
	}
	const q = `
		INSERT INTO lane_invites (
			lane_id, token, invited_by_user_id, expires_at, created_at
		) VALUES ($1,$2,$3,$4,$5)
		RETURNING id, lane_id, token, invited_by_user_id, expires_at, revoked_at, created_at
	`
	inv, err := scanLaneInvite(s.pool.QueryRow(ctx, q, laneID, token, invitedBy, expires, now))
	if err != nil {
		return models.LaneInvite{}, fmt.Errorf("create lane invite: %w", err)
	}
	return inv, nil
}

// GetLaneInviteByToken loads an invite by token.
func (s *Store) GetLaneInviteByToken(ctx context.Context, token string) (models.LaneInvite, error) {
	const q = `
		SELECT id, lane_id, token, invited_by_user_id, expires_at, revoked_at, created_at
		FROM lane_invites
		WHERE token = $1
	`
	inv, err := scanLaneInvite(s.pool.QueryRow(ctx, q, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.LaneInvite{}, ErrNotFound
	}
	if err != nil {
		return models.LaneInvite{}, fmt.Errorf("get lane invite: %w", err)
	}
	return inv, nil
}

// LaneInvitePreview is the public view of a Lane Invite (no secrets).
type LaneInvitePreview struct {
	Token                 string
	LaneID                string
	LaneName              string
	InvitedByDisplayName  string
	ExpiresAt             *time.Time
	Pending               bool
}

// GetLaneInvitePreview loads a public invite summary by token.
func (s *Store) GetLaneInvitePreview(ctx context.Context, token string) (LaneInvitePreview, error) {
	const q = `
		SELECT i.token, i.lane_id, l.name, COALESCE(u.display_name, ''),
			i.expires_at, i.revoked_at
		FROM lane_invites i
		JOIN lanes l ON l.id = i.lane_id
		LEFT JOIN users u ON u.id = i.invited_by_user_id
		WHERE i.token = $1
	`
	var (
		p         LaneInvitePreview
		revokedAt *time.Time
	)
	err := s.pool.QueryRow(ctx, q, token).Scan(
		&p.Token, &p.LaneID, &p.LaneName, &p.InvitedByDisplayName, &p.ExpiresAt, &revokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LaneInvitePreview{}, ErrNotFound
	}
	if err != nil {
		return LaneInvitePreview{}, fmt.Errorf("get lane invite preview: %w", err)
	}
	inv := models.LaneInvite{
		ExpiresAt: p.ExpiresAt,
		RevokedAt: revokedAt,
	}
	p.Pending = inv.IsPending(time.Now().UTC())
	return p, nil
}

// AcceptLaneInvite seats the user (and org membership if needed). Multi-use: does not consume.
func (s *Store) AcceptLaneInvite(ctx context.Context, token, acceptingUserID string) (models.LaneInvite, error) {
	now := time.Now().UTC()
	var out models.LaneInvite
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inv, err := scanLaneInvite(tx.QueryRow(ctx, `
			SELECT id, lane_id, token, invited_by_user_id, expires_at, revoked_at, created_at
			FROM lane_invites
			WHERE token = $1
			FOR UPDATE
		`, token))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !inv.IsPending(now) {
			return ErrConflict
		}
		return acceptLaneInviteTx(ctx, tx, inv, acceptingUserID, now, &out)
	})
	if err != nil {
		return models.LaneInvite{}, err
	}
	return out, nil
}

// LaneInviteSignupInput creates a password user and accepts an open Lane invite.
type LaneInviteSignupInput struct {
	Token            string
	Email            string
	DisplayName      string
	PasswordHash     string
	SessionToken     string
	SessionExpiresAt time.Time
}

// LaneInviteSignupResult is the new session after invite signup.
type LaneInviteSignupResult struct {
	Invite  models.LaneInvite
	User    models.User
	Session models.Session
}

// SignUpAndAcceptLaneInvite creates a user and accepts an open Lane invite.
func (s *Store) SignUpAndAcceptLaneInvite(ctx context.Context, in LaneInviteSignupInput) (LaneInviteSignupResult, error) {
	now := time.Now().UTC()
	var out LaneInviteSignupResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inv, err := scanLaneInvite(tx.QueryRow(ctx, `
			SELECT id, lane_id, token, invited_by_user_id, expires_at, revoked_at, created_at
			FROM lane_invites
			WHERE token = $1
			FOR UPDATE
		`, in.Token))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !inv.IsPending(now) {
			return ErrConflict
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1))
		`, in.Email).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrConflict
		}
		hash := in.PasswordHash
		user, err := insertUser(ctx, tx, in.Email, &hash, in.DisplayName, nil, now)
		if err != nil {
			return err
		}
		var accepted models.LaneInvite
		if err := acceptLaneInviteTx(ctx, tx, inv, user.ID, now, &accepted); err != nil {
			return err
		}
		session, err := insertSession(ctx, tx, in.SessionToken, user.ID, in.SessionExpiresAt, now)
		if err != nil {
			return err
		}
		out = LaneInviteSignupResult{Invite: accepted, User: user, Session: session}
		return nil
	})
	if err != nil {
		return LaneInviteSignupResult{}, err
	}
	return out, nil
}

// LaneInviteGoogleInput accepts (and optionally creates) a user via Google for an invite.
type LaneInviteGoogleInput struct {
	Token            string
	Email            string
	DisplayName      string
	AvatarURL        *string
	ProviderSubject  string
	SessionToken     string
	SessionExpiresAt time.Time
}

// CompleteGoogleLaneInvite creates or links a Google user and accepts the invite.
func (s *Store) CompleteGoogleLaneInvite(ctx context.Context, in LaneInviteGoogleInput) (LaneInviteSignupResult, error) {
	now := time.Now().UTC()
	var out LaneInviteSignupResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inv, err := scanLaneInvite(tx.QueryRow(ctx, `
			SELECT id, lane_id, token, invited_by_user_id, expires_at, revoked_at, created_at
			FROM lane_invites
			WHERE token = $1
			FOR UPDATE
		`, in.Token))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !inv.IsPending(now) {
			return ErrConflict
		}
		var userID string
		err = tx.QueryRow(ctx, `
			SELECT user_id FROM user_identities
			WHERE provider = $1 AND provider_subject = $2
		`, models.IdentityProviderGoogle, in.ProviderSubject).Scan(&userID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var user models.User
		if userID != "" {
			user, err = scanUser(tx.QueryRow(ctx, `
				SELECT id, email, password_hash, display_name, avatar_url, created_at, updated_at
				FROM users WHERE id = $1
			`, userID))
			if err != nil {
				return err
			}
		} else {
			existing, err := scanUser(tx.QueryRow(ctx, `
				SELECT id, email, password_hash, display_name, avatar_url, created_at, updated_at
				FROM users WHERE lower(email) = lower($1)
			`, in.Email))
			if err == nil {
				user = existing
				if err := insertUserIdentity(ctx, tx, user.ID, models.IdentityProviderGoogle, in.ProviderSubject, now); err != nil {
					if !isUniqueViolation(err) {
						return err
					}
				}
			} else if errors.Is(err, pgx.ErrNoRows) {
				user, err = insertUser(ctx, tx, in.Email, nil, in.DisplayName, in.AvatarURL, now)
				if err != nil {
					return err
				}
				if err := insertUserIdentity(ctx, tx, user.ID, models.IdentityProviderGoogle, in.ProviderSubject, now); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		var accepted models.LaneInvite
		if err := acceptLaneInviteTx(ctx, tx, inv, user.ID, now, &accepted); err != nil {
			return err
		}
		session, err := insertSession(ctx, tx, in.SessionToken, user.ID, in.SessionExpiresAt, now)
		if err != nil {
			return err
		}
		out = LaneInviteSignupResult{Invite: accepted, User: user, Session: session}
		return nil
	})
	if err != nil {
		return LaneInviteSignupResult{}, err
	}
	return out, nil
}

func acceptLaneInviteTx(
	ctx context.Context,
	tx pgx.Tx,
	inv models.LaneInvite,
	acceptingUserID string,
	now time.Time,
	out *models.LaneInvite,
) error {
	lane, err := scanLane(tx.QueryRow(ctx, `
		SELECT id, project_id, organization_id, owner_user_id, name, lane_kind,
			dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
			agent_provider_session_id, model_source, agent_model, reasoning_effort, status,
			created_at, updated_at
		FROM lanes WHERE id = $1
	`, inv.LaneID))
	if err != nil {
		return err
	}
	if lane.Status == models.LaneStatusDestroyed {
		return ErrConflict
	}
	var memberExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organization_members
			WHERE organization_id = $1 AND user_id = $2
		)
	`, lane.OrganizationID, acceptingUserID).Scan(&memberExists); err != nil {
		return err
	}
	if !memberExists {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members (organization_id, user_id, role, created_at)
			VALUES ($1, $2, $3, $4)
		`, lane.OrganizationID, acceptingUserID, models.OrganizationRoleMember, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO lane_participants (lane_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (lane_id, user_id) DO NOTHING
	`, inv.LaneID, acceptingUserID, models.LaneParticipantRoleParticipant, now); err != nil {
		return err
	}
	*out = inv
	return nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func scanLaneInvite(row scannable) (models.LaneInvite, error) {
	var inv models.LaneInvite
	err := row.Scan(
		&inv.ID, &inv.LaneID, &inv.Token, &inv.InvitedByUserID,
		&inv.ExpiresAt, &inv.RevokedAt, &inv.CreatedAt,
	)
	if err != nil {
		return models.LaneInvite{}, err
	}
	inv.CreatedAt = inv.CreatedAt.UTC()
	if inv.ExpiresAt != nil {
		u := inv.ExpiresAt.UTC()
		inv.ExpiresAt = &u
	}
	if inv.RevokedAt != nil {
		u := inv.RevokedAt.UTC()
		inv.RevokedAt = &u
	}
	return inv, nil
}
