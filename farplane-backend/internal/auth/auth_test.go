package auth

import (
	"errors"
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword(hash, "correct-horse") {
		t.Fatal("expected password to match")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected password mismatch")
	}
}

func TestHashPasswordRejectsTooLong(t *testing.T) {
	t.Parallel()

	long := string(make([]byte, MaxPasswordBytes+1))
	_, err := HashPassword(long)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("err = %v, want ErrPasswordTooLong", err)
	}
}

func TestHashSessionTokenStableAndDistinct(t *testing.T) {
	t.Parallel()

	a := HashSessionToken("token-a")
	b := HashSessionToken("token-b")
	if a == "" || a == b {
		t.Fatalf("hashes should be non-empty and distinct: %q %q", a, b)
	}
	if HashSessionToken("token-a") != a {
		t.Fatal("hash should be stable")
	}
	if a == "token-a" {
		t.Fatal("hash must not equal raw token")
	}
}

func TestOAuthStateRoundTrip(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	now := time.Unix(1_700_000_000, 0).UTC()
	state := OAuthState{
		Intent:           OAuthIntentSetup,
		OrganizationName: "Acme",
		Nonce:            "nonce",
		ExpiresAtUnix:    now.Add(5 * time.Minute).Unix(),
	}

	encoded, err := SignOAuthState(secret, state)
	if err != nil {
		t.Fatalf("SignOAuthState: %v", err)
	}

	got, err := ParseOAuthState(secret, encoded, now)
	if err != nil {
		t.Fatalf("ParseOAuthState: %v", err)
	}
	if got.Intent != state.Intent || got.OrganizationName != state.OrganizationName || got.Nonce != state.Nonce {
		t.Fatalf("got %+v, want %+v", got, state)
	}

	if _, err := ParseOAuthState("other-secret", encoded, now); err == nil {
		t.Fatal("expected invalid signature")
	}
	if _, err := ParseOAuthState(secret, encoded, now.Add(10*time.Minute)); err == nil {
		t.Fatal("expected expired state")
	}
}

func TestGitHubInstallStateRoundTrip(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	now := time.Unix(1_700_000_000, 0).UTC()
	state := GitHubInstallState{
		OrganizationID: "org-1",
		UserID:         "user-1",
		Nonce:          "nonce",
		ExpiresAtUnix:  now.Add(5 * time.Minute).Unix(),
	}

	encoded, err := SignGitHubInstallState(secret, state)
	if err != nil {
		t.Fatalf("SignGitHubInstallState: %v", err)
	}
	got, err := ParseGitHubInstallState(secret, encoded, now)
	if err != nil {
		t.Fatalf("ParseGitHubInstallState: %v", err)
	}
	if got != state {
		t.Fatalf("got %+v, want %+v", got, state)
	}
	if _, err := ParseGitHubInstallState(secret, encoded, now.Add(10*time.Minute)); err == nil {
		t.Fatal("expected expired state")
	}
}

func TestNewSessionToken(t *testing.T) {
	t.Parallel()

	a, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	b, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	if a == "" || a == b {
		t.Fatalf("tokens should be non-empty and unique: %q %q", a, b)
	}
}
