package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLength is the floor for setup and login passwords.
	MinPasswordLength = 8
	// MaxPasswordBytes is bcrypt's input limit (longer inputs are rejected).
	MaxPasswordBytes = 72
	bcryptCost       = 12
)

// ErrPasswordTooLong is returned when a password exceeds MaxPasswordBytes.
var ErrPasswordTooLong = errors.New("password too long")

// HashPassword returns a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	if len(password) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// HashSessionToken returns a hex SHA-256 digest for storing session tokens at rest.
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CheckPassword compares a plaintext password with a bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewSessionToken returns a URL-safe opaque session token.
func NewSessionToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// OAuth intents carried in signed Google state.
const (
	OAuthIntentSetup      = "setup"
	OAuthIntentLogin      = "login"
	OAuthIntentLaneInvite = "lane_invite"
)

// OAuthState is the signed payload for Google OAuth start/callback.
type OAuthState struct {
	Intent           string `json:"intent"`
	OrganizationName string `json:"organization_name,omitempty"`
	InviteToken      string `json:"invite_token,omitempty"`
	Nonce            string `json:"nonce"`
	ExpiresAtUnix    int64  `json:"exp"`
}

// ErrInvalidOAuthState is returned when state is missing, expired, or tampered.
var ErrInvalidOAuthState = errors.New("invalid oauth state")

// SignOAuthState returns a base64url payload + HMAC signature.
func SignOAuthState(secret string, state OAuthState) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("session secret is empty")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshal oauth state: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	sig := sign(secret, payload)
	return payload + "." + sig, nil
}

// ParseOAuthState verifies the signature and expiry.
func ParseOAuthState(secret, encoded string, now time.Time) (OAuthState, error) {
	payload, sig, ok := strings.Cut(encoded, ".")
	if !ok || payload == "" || sig == "" {
		return OAuthState{}, ErrInvalidOAuthState
	}
	expected := sign(secret, payload)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return OAuthState{}, ErrInvalidOAuthState
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return OAuthState{}, ErrInvalidOAuthState
	}
	var state OAuthState
	if err := json.Unmarshal(raw, &state); err != nil {
		return OAuthState{}, ErrInvalidOAuthState
	}
	if state.ExpiresAtUnix <= now.Unix() {
		return OAuthState{}, ErrInvalidOAuthState
	}
	if state.Intent != OAuthIntentSetup && state.Intent != OAuthIntentLogin {
		return OAuthState{}, ErrInvalidOAuthState
	}
	if state.Intent == OAuthIntentSetup && strings.TrimSpace(state.OrganizationName) == "" {
		return OAuthState{}, ErrInvalidOAuthState
	}
	return state, nil
}

// NewOAuthNonce returns a short random nonce for OAuth state.
func NewOAuthNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("oauth nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// GitHubInstallState is the signed payload for GitHub App install start/callback.
type GitHubInstallState struct {
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
	Nonce          string `json:"nonce"`
	ExpiresAtUnix  int64  `json:"exp"`
}

// ErrInvalidGitHubInstallState is returned when install state is bad or expired.
var ErrInvalidGitHubInstallState = errors.New("invalid github install state")

// SignGitHubInstallState returns a base64url payload + HMAC signature.
func SignGitHubInstallState(secret string, state GitHubInstallState) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("session secret is empty")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshal github install state: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	sig := sign(secret, payload)
	return payload + "." + sig, nil
}

// ParseGitHubInstallState verifies the signature and expiry.
func ParseGitHubInstallState(secret, encoded string, now time.Time) (GitHubInstallState, error) {
	payload, sig, ok := strings.Cut(encoded, ".")
	if !ok || payload == "" || sig == "" {
		return GitHubInstallState{}, ErrInvalidGitHubInstallState
	}
	expected := sign(secret, payload)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return GitHubInstallState{}, ErrInvalidGitHubInstallState
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return GitHubInstallState{}, ErrInvalidGitHubInstallState
	}
	var state GitHubInstallState
	if err := json.Unmarshal(raw, &state); err != nil {
		return GitHubInstallState{}, ErrInvalidGitHubInstallState
	}
	if state.ExpiresAtUnix <= now.Unix() {
		return GitHubInstallState{}, ErrInvalidGitHubInstallState
	}
	if strings.TrimSpace(state.OrganizationID) == "" || strings.TrimSpace(state.UserID) == "" {
		return GitHubInstallState{}, ErrInvalidGitHubInstallState
	}
	return state, nil
}

func sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
