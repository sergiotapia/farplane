package models

import "time"

// Timestamp fields map to Postgres TIMESTAMPTZ(6) (UTC, microsecond precision).
// Always read and write them as UTC time.Time values.

// Auth providers stored in user_identities.provider.
const (
	IdentityProviderGoogle = "google"
)

// Organization membership and invite roles.
const (
	OrganizationRoleOwner  = "owner"
	OrganizationRoleAdmin  = "admin"
	OrganizationRoleMember = "member"
)

// User is an account that can sign in with email/password and/or Google.
type User struct {
	ID           string
	Email        string
	PasswordHash *string // nil when the user only signs in with Google
	DisplayName  string
	AvatarURL    *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserIdentity links a user to an external sign-in provider (for example Google).
type UserIdentity struct {
	ID              string
	UserID          string
	Provider        string
	ProviderSubject string
	CreatedAt       time.Time
}

// Organization is one company or friend group on an install.
type Organization struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OrganizationMember is a user's membership in an organization.
type OrganizationMember struct {
	ID             string
	OrganizationID string
	UserID         string
	Role           string
	CreatedAt      time.Time
}

// OrganizationInvite is a shareable invite link (copy/paste URL) for an organization.
// Email may be nil for an open link that any signup can use.
type OrganizationInvite struct {
	ID               string
	OrganizationID   string
	Token            string
	Email            *string
	Role             string
	InvitedByUserID  *string
	ExpiresAt        *time.Time
	AcceptedAt       *time.Time
	AcceptedByUserID *string
	RevokedAt        *time.Time
	CreatedAt        time.Time
}

// IsPending reports whether the invite can still be accepted.
func (i OrganizationInvite) IsPending(now time.Time) bool {
	if i.AcceptedAt != nil || i.RevokedAt != nil {
		return false
	}
	if i.ExpiresAt != nil && !i.ExpiresAt.After(now) {
		return false
	}
	return true
}
