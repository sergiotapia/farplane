package agents

// ModelOption is one choosable model for a Lane agent + model source.
type ModelOption struct {
	ID                     string   `json:"id"`
	Label                  string   `json:"label"`
	ReasoningEfforts       []string `json:"reasoning_efforts"`
	DefaultReasoningEffort *string  `json:"default_reasoning_effort,omitempty"`
	SupportsReasoning      bool     `json:"supports_reasoning"`
}

// ModelSelection is a validated model_source + agent_model + optional reasoning_effort.
type ModelSelection struct {
	ModelSource     string
	AgentModel      string
	ReasoningEffort *string
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}

	out := make([]string, len(in))
	copy(out, in)

	return out
}

func cloneModelOptions(in []ModelOption) []ModelOption {
	if len(in) == 0 {
		return []ModelOption{}
	}

	out := make([]ModelOption, len(in))
	for i, opt := range in {
		out[i] = opt

		out[i].ReasoningEfforts = cloneStrings(opt.ReasoningEfforts)
		if opt.DefaultReasoningEffort != nil {
			out[i].DefaultReasoningEffort = stringPtr(*opt.DefaultReasoningEffort)
		}
	}

	return out
}
