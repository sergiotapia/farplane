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
		switch a.Provider {
		case models.AgentProviderClaudeCode, models.AgentProviderOpenCode, models.AgentProviderOhMyPi:
			if !a.Available {
				t.Fatalf("%s should be available with anthropic key", a.Provider)
			}
		case models.AgentProviderCodex:
			if a.Available {
				t.Fatalf("codex should not be available without openai key")
			}
		}
	}
}

func TestAvailabilityOpenRouterUnlocksMultiProviderAgents(t *testing.T) {
	t.Parallel()

	got := agents.Availability(map[string]bool{
		models.SecretNameOpenRouterAPIKey: true,
	})
	for _, a := range got {
		want := a.Provider == models.AgentProviderOpenCode ||
			a.Provider == models.AgentProviderOhMyPi
		if a.Available != want {
			t.Fatalf("%s available = %v, want %v", a.Provider, a.Available, want)
		}

		if a.Available && len(a.ModelSources) != 1 {
			t.Fatalf("%s sources = %#v", a.Provider, a.ModelSources)
		}
	}
}

func TestSourcesForAgent(t *testing.T) {
	t.Parallel()

	secrets := map[string]bool{
		models.SecretNameAnthropicAPIKey:  true,
		models.SecretNameOpenRouterAPIKey: true,
	}

	sources := agents.SourcesForAgent(models.AgentProviderOhMyPi, secrets)
	if len(sources) != 2 {
		t.Fatalf("sources = %#v", sources)
	}

	if sources[0].ID != agents.ModelSourceOpenRouter {
		t.Fatalf("prefer openrouter first, got %s", sources[0].ID)
	}

	if sources[1].ID != agents.ModelSourceAnthropic {
		t.Fatalf("second = %s", sources[1].ID)
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
