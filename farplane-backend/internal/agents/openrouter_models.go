package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	openRouterModelsURL   = "https://openrouter.ai/api/v1/models"
	openRouterCacheTTL    = time.Hour
	openRouterHTTPTimeout = 15 * time.Second
)

// openRouterAPIModel is the subset of OpenRouter /models fields we need.
type openRouterAPIModel struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	SupportedParameters []string `json:"supported_parameters"`
	Architecture        struct {
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Reasoning *struct {
		SupportedEfforts []string `json:"supported_efforts"`
		DefaultEffort    string   `json:"default_effort"`
	} `json:"reasoning"`
}

type openRouterModelsResponse struct {
	Data []openRouterAPIModel `json:"data"`
}

// HTTPDoer is the subset of http.Client used for OpenRouter fetches (tests inject fakes).
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// OpenRouterModelsClient fetches and caches OpenRouter model options.
type OpenRouterModelsClient struct {
	httpClient HTTPDoer
	ttl        time.Duration
	now        func() time.Time

	mu        sync.Mutex
	cached    []ModelOption
	fetchedAt time.Time
	hasCache  bool
}

// NewOpenRouterModelsClient builds a client with a 1h in-process cache.
func NewOpenRouterModelsClient(httpClient HTTPDoer) *OpenRouterModelsClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: openRouterHTTPTimeout}
	}

	return &OpenRouterModelsClient{
		httpClient: httpClient,
		ttl:        openRouterCacheTTL,
		now:        time.Now,
	}
}

// SetClockForTest overrides the clock used for cache TTL (tests only).
func (c *OpenRouterModelsClient) SetClockForTest(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = now
}

// SetTTLForTest overrides the cache TTL (tests only).
func (c *OpenRouterModelsClient) SetTTLForTest(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ttl = ttl
}

// List returns filtered OpenRouter models suitable for coding agents.
// On fetch failure, returns the last good cache when present.
func (c *OpenRouterModelsClient) List(ctx context.Context) ([]ModelOption, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasCache && c.now().Sub(c.fetchedAt) < c.ttl {
		return cloneModelOptions(c.cached), nil
	}

	models, err := c.fetch(ctx)
	if err != nil {
		if c.hasCache {
			return cloneModelOptions(c.cached), nil
		}

		return nil, err
	}

	c.cached = models
	c.fetchedAt = c.now()
	c.hasCache = true

	return cloneModelOptions(c.cached), nil
}

func (c *OpenRouterModelsClient) fetch(ctx context.Context) ([]ModelOption, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("openrouter models request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter models fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("openrouter models read: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter models status %d", resp.StatusCode)
	}

	var parsed openRouterModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("openrouter models decode: %w", err)
	}

	return mapOpenRouterModels(parsed.Data), nil
}

// mapOpenRouterModels filters for tools + text and maps to ModelOption.
func mapOpenRouterModels(in []openRouterAPIModel) []ModelOption {
	out := make([]ModelOption, 0, len(in))
	for _, m := range in {
		if !supportsParameter(m.SupportedParameters, "tools") {
			continue
		}

		if !hasTextOutput(m.Architecture.OutputModalities) {
			continue
		}

		opt := ModelOption{
			ID:    m.ID,
			Label: openRouterDisplayLabel(m.Name, m.ID),
		}
		if m.Reasoning != nil {
			efforts := cloneStrings(m.Reasoning.SupportedEfforts)
			opt.ReasoningEfforts = efforts

			opt.SupportsReasoning = len(efforts) > 0
			if def := strings.TrimSpace(m.Reasoning.DefaultEffort); def != "" {
				opt.DefaultReasoningEffort = stringPtr(def)
			} else if len(efforts) > 0 {
				opt.DefaultReasoningEffort = stringPtr(efforts[0])
			}
		} else {
			opt.ReasoningEfforts = []string{}
		}

		out = append(out, opt)
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})

	return out
}

// openRouterDisplayLabel strips a leading "Author: " prefix from OpenRouter names.
func openRouterDisplayLabel(name, id string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return id
	}

	if _, after, ok := strings.Cut(name, ": "); ok {
		rest := strings.TrimSpace(after)
		if rest != "" {
			return rest
		}
	}

	return name
}

func supportsParameter(params []string, want string) bool {
	return slices.Contains(params, want)
}

func hasTextOutput(modalities []string) bool {
	if len(modalities) == 0 {
		// OpenRouter defaults the list endpoint to text; treat missing as text.
		return true
	}

	return slices.Contains(modalities, "text")
}
