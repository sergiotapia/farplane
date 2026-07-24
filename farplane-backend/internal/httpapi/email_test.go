package httpapi

import (
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/config"
)

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	got, ok := normalizeEmail("Owner@Example.com")
	if !ok || got != "owner@example.com" {
		t.Fatalf("got %q ok=%v", got, ok)
	}

	if _, ok := normalizeEmail("Alice <alice@example.com>"); ok {
		t.Fatal("display-name email should be rejected")
	}

	if _, ok := normalizeEmail(""); ok {
		t.Fatal("empty email should be rejected")
	}
}

func TestSpaOriginsIncludesLocalhostAliases(t *testing.T) {
	t.Parallel()

	origins := spaOrigins(config.Config{AppBaseURL: "http://localhost:3000"})

	want := map[string]bool{
		"http://localhost:3000": true,
		"http://127.0.0.1:3000": true,
	}
	for _, origin := range origins {
		delete(want, origin)
	}

	if len(want) != 0 {
		t.Fatalf("missing origins: %#v (have %#v)", want, origins)
	}
}
