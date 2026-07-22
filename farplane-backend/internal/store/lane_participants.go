package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// ListLaneParticipants returns all seats (including removed) with optional active-only filter.
func (s *Store) ListLaneParticipants(ctx context.Context, laneID string, activeOnly bool) ([]models.LaneParticipant, error) {
	q := `
		SELECT id, lane_id, user_id, role, joined_at, removed_at, removed_by_user_id
		FROM lane_participants
		WHERE lane_id = $1
	`
	if activeOnly {
		q += ` AND removed_at IS NULL`
	}
	q += ` ORDER BY joined_at ASC`
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

// ListOrganizationMembersForInvite returns org members for Lane invite autocomplete.
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

// CreateLaneInviteInput creates a pending invite.
type CreateLaneInviteInput struct {
	LaneID          string
	Email           *string
	InvitedUserID   *string
	InvitedByUserID string
	ExpiresAt       *time.Time
}

// CreateLaneInvite inserts a pending invite with a random token.
func (s *Store) CreateLaneInvite(ctx context.Context, in CreateLaneInviteInput) (models.LaneInvite, error) {
	if in.Email == nil && in.InvitedUserID == nil {
		return models.LaneInvite{}, fmt.Errorf("email or invited_user_id is required")
	}
	token, err := randomToken(32)
	if err != nil {
		return models.LaneInvite{}, err
	}
	now := time.Now().UTC()
	var invitedBy *string
	if in.InvitedByUserID != "" {
		invitedBy = &in.InvitedByUserID
	}
	var email *string
	if in.Email != nil {
		e := strings.TrimSpace(strings.ToLower(*in.Email))
		email = &e
	}
	const q = `
		INSERT INTO lane_invites (
			lane_id, token, email, invited_user_id, invited_by_user_id, expires_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, lane_id, token, email, invited_user_id, invited_by_user_id,
			expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
	`
	inv, err := scanLaneInvite(s.pool.QueryRow(
		ctx, q, in.LaneID, token, email, in.InvitedUserID, invitedBy, in.ExpiresAt, now,
	))
	if err != nil {
		return models.LaneInvite{}, fmt.Errorf("create lane invite: %w", err)
	}
	return inv, nil
}

// ListLaneInvites returns invites for a lane.
func (s *Store) ListLaneInvites(ctx context.Context, laneID string) ([]models.LaneInvite, error) {
	const q = `
		SELECT id, lane_id, token, email, invited_user_id, invited_by_user_id,
			expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
		FROM lane_invites
		WHERE lane_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, q, laneID)
	if err != nil {
		return nil, fmt.Errorf("list lane invites: %w", err)
	}
	defer rows.Close()
	var out []models.LaneInvite
	for rows.Next() {
		inv, err := scanLaneInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// GetLaneInviteByToken loads an invite by token.
func (s *Store) GetLaneInviteByToken(ctx context.Context, token string) (models.LaneInvite, error) {
	const q = `
		SELECT id, lane_id, token, email, invited_user_id, invited_by_user_id,
			expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
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

// LaneInvitePreview is the public view of a pending invite (no secrets).
type LaneInvitePreview struct {
	Token     string
	LaneID    string
	LaneName  string
	Email     *string
	ExpiresAt *time.Time
	Pending   bool
}

// GetLaneInvitePreview loads a public invite summary by token.
func (s *Store) GetLaneInvitePreview(ctx context.Context, token string) (LaneInvitePreview, error) {
	const q = `
		SELECT i.token, i.lane_id, l.name, i.email, i.expires_at,
			i.accepted_at, i.revoked_at
		FROM lane_invites i
		JOIN lanes l ON l.id = i.lane_id
		WHERE i.token = $1
	`
	var (
		p          LaneInvitePreview
		acceptedAt *time.Time
		revokedAt  *time.Time
	)
	err := s.pool.QueryRow(ctx, q, token).Scan(
		&p.Token, &p.LaneID, &p.LaneName, &p.Email, &p.ExpiresAt, &acceptedAt, &revokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LaneInvitePreview{}, ErrNotFound
	}
	if err != nil {
		return LaneInvitePreview{}, fmt.Errorf("get lane invite preview: %w", err)
	}
	inv := models.LaneInvite{
		ExpiresAt:  p.ExpiresAt,
		AcceptedAt: acceptedAt,
		RevokedAt:  revokedAt,
	}
	p.Pending = inv.IsPending(time.Now().UTC())
	return p, nil
}

// AcceptLaneInvite marks the invite accepted and upserts an active participant.
func (s *Store) AcceptLaneInvite(ctx context.Context, token, acceptingUserID string) (models.LaneInvite, error) {
	now := time.Now().UTC()
	var out models.LaneInvite
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inv, err := scanLaneInvite(tx.QueryRow(ctx, `
			SELECT id, lane_id, token, email, invited_user_id, invited_by_user_id,
				expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
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
		if inv.InvitedUserID != nil && *inv.InvitedUserID != acceptingUserID {
			return ErrForbidden
		}
		user, err := scanUser(tx.QueryRow(ctx, `
			SELECT id, email, password_hash, display_name, avatar_url, created_at, updated_at
			FROM users WHERE id = $1
		`, acceptingUserID))
		if err != nil {
			return err
		}
		if inv.Email != nil && strings.TrimSpace(*inv.Email) != "" {
			if !strings.EqualFold(user.Email, strings.TrimSpace(*inv.Email)) {
				return ErrForbidden
			}
		}
		return acceptLaneInviteTx(ctx, tx, inv, acceptingUserID, now, &out)
	})
	if err != nil {
		return models.LaneInvite{}, err
	}
	return out, nil
}

// LaneInviteSignupInput creates a password user and accepts an email Lane invite.
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

// SignUpAndAcceptLaneInvite creates a user for an email invite and accepts it.
func (s *Store) SignUpAndAcceptLaneInvite(ctx context.Context, in LaneInviteSignupInput) (LaneInviteSignupResult, error) {
	now := time.Now().UTC()
	var out LaneInviteSignupResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		inv, err := scanLaneInvite(tx.QueryRow(ctx, `
			SELECT id, lane_id, token, email, invited_user_id, invited_by_user_id,
				expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
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
		if inv.Email == nil || strings.TrimSpace(*inv.Email) == "" {
			return ErrForbidden
		}
		if !strings.EqualFold(strings.TrimSpace(*inv.Email), in.Email) {
			return ErrForbidden
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
			SELECT id, lane_id, token, email, invited_user_id, invited_by_user_id,
				expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
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
		if inv.Email != nil && strings.TrimSpace(*inv.Email) != "" {
			if !strings.EqualFold(strings.TrimSpace(*inv.Email), in.Email) {
				return ErrForbidden
			}
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
		if inv.InvitedUserID != nil && *inv.InvitedUserID != user.ID {
			return ErrForbidden
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
		SELECT id, project_id, organization_id, owner_user_id, name, lane_template_id,
			dockerfile_snapshot, image_reference, runtime_kind, runtime_id, agent_provider,
			agent_provider_session_id, status, created_at, updated_at
		FROM lanes WHERE id = $1
	`, inv.LaneID))
	if err != nil {
		return err
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
		ON CONFLICT (lane_id, user_id) DO UPDATE SET
			removed_at = NULL,
			removed_by_user_id = NULL,
			joined_at = EXCLUDED.joined_at,
			role = CASE
				WHEN lane_participants.role = 'owner' THEN lane_participants.role
				ELSE EXCLUDED.role
			END
	`, inv.LaneID, acceptingUserID, models.LaneParticipantRoleParticipant, now); err != nil {
		return err
	}
	updated, err := scanLaneInvite(tx.QueryRow(ctx, `
		UPDATE lane_invites
		SET accepted_at = $2, accepted_by_user_id = $3
		WHERE id = $1
		RETURNING id, lane_id, token, email, invited_user_id, invited_by_user_id,
			expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
	`, inv.ID, now, acceptingUserID))
	if err != nil {
		return err
	}
	*out = updated
	return nil
}

// KickLaneParticipant soft-removes a participant and revokes pending invites for them.
func (s *Store) KickLaneParticipant(ctx context.Context, laneID, targetUserID, kickedByUserID string) error {
	now := time.Now().UTC()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var role string
		err := tx.QueryRow(ctx, `
			SELECT role FROM lane_participants
			WHERE lane_id = $1 AND user_id = $2 AND removed_at IS NULL
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
		tag, err := tx.Exec(ctx, `
			UPDATE lane_participants
			SET removed_at = $3, removed_by_user_id = $4
			WHERE lane_id = $1 AND user_id = $2 AND removed_at IS NULL
		`, laneID, targetUserID, now, kickedByUserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `
			UPDATE lane_invites
			SET revoked_at = $3
			WHERE lane_id = $1 AND invited_user_id = $2
				AND accepted_at IS NULL AND revoked_at IS NULL
		`, laneID, targetUserID, now)
		return err
	})
}

// RevokeLaneInvite revokes a pending invite.
func (s *Store) RevokeLaneInvite(ctx context.Context, inviteID, laneID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE lane_invites
		SET revoked_at = $3
		WHERE id = $1 AND lane_id = $2 AND accepted_at IS NULL AND revoked_at IS NULL
	`, inviteID, laneID, time.Now().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
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
		&inv.ID, &inv.LaneID, &inv.Token, &inv.Email, &inv.InvitedUserID, &inv.InvitedByUserID,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.AcceptedByUserID, &inv.RevokedAt, &inv.CreatedAt,
	)
	if err != nil {
		return models.LaneInvite{}, err
	}
	inv.CreatedAt = inv.CreatedAt.UTC()
	if inv.ExpiresAt != nil {
		u := inv.ExpiresAt.UTC()
		inv.ExpiresAt = &u
	}
	if inv.AcceptedAt != nil {
		u := inv.AcceptedAt.UTC()
		inv.AcceptedAt = &u
	}
	if inv.RevokedAt != nil {
		u := inv.RevokedAt.UTC()
		inv.RevokedAt = &u
	}
	return inv, nil
}
