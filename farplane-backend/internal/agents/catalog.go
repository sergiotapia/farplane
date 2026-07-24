package agents

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
)

// ModelCatalog resolves model options per agent provider + model source.
type ModelCatalog struct {
	openRouter *OpenRouterModelsClient
}

var (
	defaultModelCatalog     *ModelCatalog
	defaultModelCatalogOnce sync.Once
)

// DefaultModelCatalog returns the process-wide catalog (shared OpenRouter cache).
func DefaultModelCatalog() *ModelCatalog {
	defaultModelCatalogOnce.Do(func() {
		defaultModelCatalog = NewModelCatalog(NewOpenRouterModelsClient(nil))
	})

	return defaultModelCatalog
}

// NewModelCatalog builds a catalog with the given OpenRouter client (tests inject fakes).
func NewModelCatalog(openRouter *OpenRouterModelsClient) *ModelCatalog {
	if openRouter == nil {
		openRouter = NewOpenRouterModelsClient(nil)
	}

	return &ModelCatalog{openRouter: openRouter}
}

// ModelsFor returns model options for an agent provider and model source.
func (c *ModelCatalog) ModelsFor(ctx context.Context, provider, source string) ([]ModelOption, error) {
	provider = strings.TrimSpace(provider)
	source = strings.TrimSpace(source)

	if !IsKnownProvider(provider) {
		return nil, errors.New("unknown agent provider")
	}

	if !IsKnownModelSource(source) {
		return nil, errors.New("unknown model_source")
	}

	allowed := slices.Contains(supportedSourcesForAgent(provider), source)

	if !allowed {
		return nil, errors.New("model_source not supported for this agent")
	}

	if models, ok := staticModelsForAgent(provider, source); ok {
		return models, nil
	}

	if source == ModelSourceOpenRouter {
		return c.openRouter.List(ctx)
	}

	return nil, errors.New("unknown model_source")
}

// DefaultSelection returns a sensible source + model + effort for an agent
// given which secrets are set.
func (c *ModelCatalog) DefaultSelection(
	ctx context.Context,
	provider string,
	setSecrets map[string]bool,
) (ModelSelection, error) {
	source, ok := DefaultModelSource(provider, setSecrets)
	if !ok {
		return ModelSelection{}, errors.New("no model source available for agent")
	}

	return c.DefaultSelectionForSource(ctx, provider, source)
}

// DefaultSelectionForSource returns the default model for a fixed source.
func (c *ModelCatalog) DefaultSelectionForSource(
	ctx context.Context,
	provider, source string,
) (ModelSelection, error) {
	list, err := c.ModelsFor(ctx, provider, source)
	if err != nil {
		return ModelSelection{}, err
	}

	if len(list) == 0 {
		return ModelSelection{}, errors.New("no models available for model_source")
	}

	sel := selectionFromOption(list[0])
	sel.ModelSource = source

	return sel, nil
}

// ValidateSelection checks model_source, agent_model, and reasoning_effort.
func (c *ModelCatalog) ValidateSelection( //nolint:gocyclo // multi-branch orchestration; keep under threshold when rewriting
	ctx context.Context,
	provider, source, agentModel string,
	reasoningEffort *string,
) (ModelSelection, error) {
	provider = strings.TrimSpace(provider)
	source = strings.TrimSpace(source)
	agentModel = strings.TrimSpace(agentModel)

	if source == "" {
		return ModelSelection{}, errors.New("model_source is required")
	}

	if agentModel == "" {
		return ModelSelection{}, errors.New("agent_model is required")
	}

	list, err := c.ModelsFor(ctx, provider, source)
	if err != nil {
		return ModelSelection{}, err
	}

	var opt *ModelOption

	for i := range list {
		if list[i].ID == agentModel {
			opt = &list[i]
			break
		}
	}

	if opt == nil {
		return ModelSelection{}, errors.New("unknown agent_model for model_source")
	}

	effort := ""
	if reasoningEffort != nil {
		effort = strings.TrimSpace(*reasoningEffort)
	}

	if len(opt.ReasoningEfforts) == 0 {
		return ModelSelection{
			ModelSource:     source,
			AgentModel:      opt.ID,
			ReasoningEffort: nil,
		}, nil
	}

	if effort == "" {
		if opt.DefaultReasoningEffort != nil {
			effort = *opt.DefaultReasoningEffort
		} else {
			effort = opt.ReasoningEfforts[0]
		}
	}

	if !containsString(opt.ReasoningEfforts, effort) {
		return ModelSelection{}, errors.New("invalid reasoning_effort for this model")
	}

	return ModelSelection{
		ModelSource:     source,
		AgentModel:      opt.ID,
		ReasoningEffort: stringPtr(effort),
	}, nil
}

// ResolveForAgentChange returns selection after an agent switch.
// Keeps source/model/effort when still valid for the new agent; otherwise defaults.
func (c *ModelCatalog) ResolveForAgentChange(
	ctx context.Context,
	provider, currentSource, currentModel string,
	currentEffort *string,
	setSecrets map[string]bool,
) (ModelSelection, error) {
	if currentSource != "" && SourceAllowedForAgent(provider, currentSource, setSecrets) {
		if currentModel != "" {
			sel, err := c.ValidateSelection(ctx, provider, currentSource, currentModel, currentEffort)
			if err == nil {
				return sel, nil
			}
		}

		return c.DefaultSelectionForSource(ctx, provider, currentSource)
	}

	return c.DefaultSelection(ctx, provider, setSecrets)
}

// ResolveForSourceChange returns selection after only model_source changes.
func (c *ModelCatalog) ResolveForSourceChange(
	ctx context.Context,
	provider, source, currentModel string,
	currentEffort *string,
) (ModelSelection, error) {
	if currentModel != "" {
		sel, err := c.ValidateSelection(ctx, provider, source, currentModel, currentEffort)
		if err == nil {
			return sel, nil
		}
	}

	return c.DefaultSelectionForSource(ctx, provider, source)
}

func selectionFromOption(opt ModelOption) ModelSelection {
	sel := ModelSelection{AgentModel: opt.ID}
	if len(opt.ReasoningEfforts) == 0 {
		return sel
	}

	if opt.DefaultReasoningEffort != nil {
		sel.ReasoningEffort = stringPtr(*opt.DefaultReasoningEffort)
		return sel
	}

	sel.ReasoningEffort = stringPtr(opt.ReasoningEfforts[0])

	return sel
}

func containsString(list []string, want string) bool {
	return slices.Contains(list, want)
}
