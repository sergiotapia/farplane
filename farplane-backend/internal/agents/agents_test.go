package agents_test

import (
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

func TestAvailability(t *testing.T) {
	t.Parallel()
	got := agents.Availability(map[string]bool{
		models.SecretNameAnthropicAPIKey: true,
	})
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	for _, a := range got {
		want := a.Provider == models.AgentProviderClaudeCode
		if a.Available != want {
			t.Fatalf("%s available = %v, want %v", a.Provider, a.Available, want)
		}
	}
}

func TestRequiredSecretFor(t *testing.T) {
	t.Parallel()
	if agents.RequiredSecretFor(models.AgentProviderCodex) != models.SecretNameOpenAIAPIKey {
		t.Fatal("codex secret mismatch")
	}
	if agents.RequiredSecretFor("nope") != "" {
		t.Fatal("unknown provider should return empty")
	}
}
