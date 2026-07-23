package agents_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

func TestValidateSelectionStaticCatalog(t *testing.T) {
	t.Parallel()
	catalog := agents.NewModelCatalog(agents.NewOpenRouterModelsClient(nil))

	medium := "medium"
	sel, err := catalog.ValidateSelection(
		context.Background(),
		models.AgentProviderClaudeCode,
		agents.ModelSourceAnthropic,
		"claude-sonnet-4-5",
		&medium,
	)
	if err != nil {
		t.Fatalf("ValidateSelection: %v", err)
	}
	if sel.ModelSource != agents.ModelSourceAnthropic {
		t.Fatalf("source = %s", sel.ModelSource)
	}
	if sel.AgentModel != "claude-sonnet-4-5" {
		t.Fatalf("model = %s", sel.AgentModel)
	}
	if sel.ReasoningEffort == nil || *sel.ReasoningEffort != "medium" {
		t.Fatalf("effort = %#v", sel.ReasoningEffort)
	}

	_, err = catalog.ValidateSelection(
		context.Background(),
		models.AgentProviderClaudeCode,
		agents.ModelSourceAnthropic,
		"not-a-model",
		&medium,
	)
	if err == nil {
		t.Fatal("expected unknown model error")
	}

	bogus := "bogus"
	_, err = catalog.ValidateSelection(
		context.Background(),
		models.AgentProviderCodex,
		agents.ModelSourceOpenAI,
		"gpt-5.1-codex",
		&bogus,
	)
	if err == nil {
		t.Fatal("expected invalid effort error")
	}
}

func TestResolveForAgentChangeResetsInvalidModel(t *testing.T) {
	t.Parallel()
	catalog := agents.NewModelCatalog(agents.NewOpenRouterModelsClient(nil))
	secrets := map[string]bool{
		models.SecretNameAnthropicAPIKey:  true,
		models.SecretNameOpenAIAPIKey:     true,
		models.SecretNameOpenRouterAPIKey: true,
	}

	medium := "medium"
	sel, err := catalog.ResolveForAgentChange(
		context.Background(),
		models.AgentProviderCodex,
		agents.ModelSourceAnthropic,
		"claude-sonnet-4-5",
		&medium,
		secrets,
	)
	if err != nil {
		t.Fatalf("ResolveForAgentChange: %v", err)
	}
	if sel.ModelSource != agents.ModelSourceOpenAI {
		t.Fatalf("reset source = %s", sel.ModelSource)
	}
	if sel.AgentModel != "gpt-5.1-codex" {
		t.Fatalf("reset model = %s, want gpt-5.1-codex", sel.AgentModel)
	}

	// o3 is OpenAI-direct but not in the Codex-only list → reset.
	resetFromO3, err := catalog.ResolveForAgentChange(
		context.Background(),
		models.AgentProviderCodex,
		agents.ModelSourceOpenAI,
		"o3",
		&medium,
		secrets,
	)
	if err != nil {
		t.Fatalf("reset ResolveForAgentChange: %v", err)
	}
	if resetFromO3.AgentModel != "gpt-5.1-codex" {
		t.Fatalf("reset from o3 = %s, want gpt-5.1-codex", resetFromO3.AgentModel)
	}

	kept, err := catalog.ResolveForAgentChange(
		context.Background(),
		models.AgentProviderCodex,
		agents.ModelSourceOpenAI,
		"gpt-5.1-codex-mini",
		&medium,
		secrets,
	)
	if err != nil {
		t.Fatalf("keep ResolveForAgentChange: %v", err)
	}
	if kept.AgentModel != "gpt-5.1-codex-mini" {
		t.Fatalf("kept model = %s", kept.AgentModel)
	}
}

func TestResolveForSourceChangeResetsModel(t *testing.T) {
	t.Parallel()
	catalog := agents.NewModelCatalog(agents.NewOpenRouterModelsClient(nil))
	medium := "medium"
	sel, err := catalog.ResolveForSourceChange(
		context.Background(),
		models.AgentProviderOhMyPi,
		agents.ModelSourceAnthropic,
		"openai/gpt-test",
		&medium,
	)
	if err != nil {
		t.Fatalf("ResolveForSourceChange: %v", err)
	}
	if sel.ModelSource != agents.ModelSourceAnthropic {
		t.Fatalf("source = %s", sel.ModelSource)
	}
	if sel.AgentModel != "claude-sonnet-4-5" {
		t.Fatalf("model = %s", sel.AgentModel)
	}
}

func TestDefaultSelectionClaudeAndCodex(t *testing.T) {
	t.Parallel()
	catalog := agents.NewModelCatalog(agents.NewOpenRouterModelsClient(nil))
	secrets := map[string]bool{
		models.SecretNameAnthropicAPIKey: true,
		models.SecretNameOpenAIAPIKey:    true,
	}

	claude, err := catalog.DefaultSelection(
		context.Background(), models.AgentProviderClaudeCode, secrets,
	)
	if err != nil {
		t.Fatalf("claude default: %v", err)
	}
	if claude.ModelSource != agents.ModelSourceAnthropic || claude.AgentModel != "claude-sonnet-4-5" {
		t.Fatalf("claude default = %#v", claude)
	}

	codex, err := catalog.DefaultSelection(
		context.Background(), models.AgentProviderCodex, secrets,
	)
	if err != nil {
		t.Fatalf("codex default: %v", err)
	}
	if codex.ModelSource != agents.ModelSourceOpenAI || codex.AgentModel != "gpt-5.1-codex" {
		t.Fatalf("codex default = %#v", codex)
	}
}

func TestModelsForOpenRouterUsesClient(t *testing.T) {
	t.Parallel()
	payload := `{
		"data": [{
			"id": "openai/gpt-test",
			"name": "OpenAI: GPT Test",
			"supported_parameters": ["tools"],
			"architecture": {"output_modalities": ["text"]},
			"reasoning": {"supported_efforts": ["none", "high"], "default_effort": "high"}
		}]
	}`
	client := agents.NewOpenRouterModelsClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Header:     make(http.Header),
		}, nil
	}))
	catalog := agents.NewModelCatalog(client)

	list, err := catalog.ModelsFor(
		context.Background(), models.AgentProviderOpenCode, agents.ModelSourceOpenRouter,
	)
	if err != nil {
		t.Fatalf("ModelsFor: %v", err)
	}
	if len(list) != 1 || list[0].ID != "openai/gpt-test" {
		t.Fatalf("list = %#v", list)
	}
	if list[0].Label != "GPT Test" {
		t.Fatalf("label = %q, want stripped org prefix", list[0].Label)
	}

	high := "high"
	sel, err := catalog.ValidateSelection(
		context.Background(),
		models.AgentProviderOhMyPi,
		agents.ModelSourceOpenRouter,
		"openai/gpt-test",
		&high,
	)
	if err != nil {
		t.Fatalf("ValidateSelection openrouter: %v", err)
	}
	if sel.AgentModel != "openai/gpt-test" {
		t.Fatalf("model = %s", sel.AgentModel)
	}
}

func TestOhMyPiAnthropicDirectModels(t *testing.T) {
	t.Parallel()
	catalog := agents.NewModelCatalog(agents.NewOpenRouterModelsClient(nil))
	list, err := catalog.ModelsFor(
		context.Background(), models.AgentProviderOhMyPi, agents.ModelSourceAnthropic,
	)
	if err != nil {
		t.Fatalf("ModelsFor: %v", err)
	}
	if len(list) < 1 || list[0].ID != "claude-sonnet-4-5" {
		t.Fatalf("list = %#v", list)
	}
}

func TestCodexUsesCodexOnlyOpenAIModels(t *testing.T) {
	t.Parallel()
	catalog := agents.NewModelCatalog(agents.NewOpenRouterModelsClient(nil))
	codexList, err := catalog.ModelsFor(
		context.Background(), models.AgentProviderCodex, agents.ModelSourceOpenAI,
	)
	if err != nil {
		t.Fatalf("codex ModelsFor: %v", err)
	}
	openCodeList, err := catalog.ModelsFor(
		context.Background(), models.AgentProviderOpenCode, agents.ModelSourceOpenAI,
	)
	if err != nil {
		t.Fatalf("opencode ModelsFor: %v", err)
	}
	if len(codexList) != 2 {
		t.Fatalf("codex list len = %d, want 2", len(codexList))
	}
	if len(openCodeList) <= len(codexList) {
		t.Fatalf("opencode openai list should be wider than codex: %d vs %d", len(openCodeList), len(codexList))
	}
	for _, opt := range codexList {
		if !strings.Contains(opt.ID, "codex") {
			t.Fatalf("codex catalog includes non-codex model %q", opt.ID)
		}
	}
}
