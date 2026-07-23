package agents_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestMapOpenRouterModelsFiltersToolsAndText(t *testing.T) {
	t.Parallel()
	payload := `{
		"data": [
			{
				"id": "keep/tools-text",
				"name": "Keep Me",
				"supported_parameters": ["tools", "temperature"],
				"architecture": {"output_modalities": ["text"]},
				"reasoning": {
					"supported_efforts": ["low", "medium", "high"],
					"default_effort": "medium"
				}
			},
			{
				"id": "drop/no-tools",
				"name": "No Tools",
				"supported_parameters": ["temperature"],
				"architecture": {"output_modalities": ["text"]}
			},
			{
				"id": "drop/image-only",
				"name": "Image Only",
				"supported_parameters": ["tools"],
				"architecture": {"output_modalities": ["image"]}
			},
			{
				"id": "keep/no-reasoning",
				"name": "AAA No Reasoning",
				"supported_parameters": ["tools"],
				"architecture": {"output_modalities": ["text"]}
			}
		]
	}`

	var calls atomic.Int32
	client := agents.NewOpenRouterModelsClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.String() != "https://openrouter.ai/api/v1/models" {
			t.Fatalf("url = %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Header:     make(http.Header),
		}, nil
	}))

	got, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	// Sorted by label.
	if got[0].ID != "keep/no-reasoning" || got[1].ID != "keep/tools-text" {
		t.Fatalf("ids = %s, %s", got[0].ID, got[1].ID)
	}
	if got[0].SupportsReasoning || len(got[0].ReasoningEfforts) != 0 {
		t.Fatalf("no-reasoning option = %#v", got[0])
	}
	if !got[1].SupportsReasoning {
		t.Fatal("expected supports_reasoning")
	}
	if got[1].DefaultReasoningEffort == nil || *got[1].DefaultReasoningEffort != "medium" {
		t.Fatalf("default effort = %#v", got[1].DefaultReasoningEffort)
	}
	if strings.Join(got[1].ReasoningEfforts, ",") != "low,medium,high" {
		t.Fatalf("efforts = %#v", got[1].ReasoningEfforts)
	}
}

func TestOpenRouterCacheTTLAndStaleFallback(t *testing.T) {
	t.Parallel()
	payload := `{
		"data": [{
			"id": "keep/a",
			"name": "A",
			"supported_parameters": ["tools"],
			"architecture": {"output_modalities": ["text"]}
		}]
	}`
	var calls atomic.Int32
	var now atomic.Int64
	now.Store(time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC).UnixNano())

	client := agents.NewOpenRouterModelsClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := calls.Add(1)
		if n == 1 {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(payload)),
				Header:     make(http.Header),
			}, nil
		}
		return nil, errors.New("upstream down")
	}))
	client.SetClockForTest(func() time.Time {
		return time.Unix(0, now.Load()).UTC()
	})
	client.SetTTLForTest(time.Hour)

	first, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("first List: %v", err)
	}
	if len(first) != 1 || first[0].ID != "keep/a" {
		t.Fatalf("first = %#v", first)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls after first = %d", calls.Load())
	}

	// Within TTL: no refetch.
	second, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if len(second) != 1 || calls.Load() != 1 {
		t.Fatalf("second calls=%d second=%#v", calls.Load(), second)
	}

	// Past TTL: fetch fails, soft-fail to last good cache.
	now.Store(time.Unix(0, now.Load()).Add(2 * time.Hour).UnixNano())
	third, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("stale fallback List: %v", err)
	}
	if len(third) != 1 || third[0].ID != "keep/a" {
		t.Fatalf("third = %#v", third)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls after stale = %d, want 2", calls.Load())
	}
}
