package envgen

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

func TestStripMarkdownFenceRapid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		body := rapid.String().Draw(t, "body")
		lang := rapid.SampledFrom([]string{"", "dockerfile", "Dockerfile"}).Draw(t, "lang")
		fenced := "```" + lang + "\n" + body + "\n```"

		got := stripMarkdownFence(fenced)
		if strings.HasPrefix(strings.TrimSpace(got), "```") {
			t.Fatalf("fence not stripped: %q", got)
		}

		unfenced := rapid.String().Filter(func(s string) bool {
			return !strings.HasPrefix(strings.TrimSpace(s), "```")
		}).Draw(t, "plain")
		if stripMarkdownFence(unfenced) != strings.TrimSpace(unfenced) {
			t.Fatalf("plain text changed unexpectedly")
		}
	})
}

func TestTruncateRapid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "s")
		n := rapid.IntRange(0, 64).Draw(t, "n")

		got := truncate(s, n)
		if n >= len(s) {
			if got != s {
				t.Fatalf("truncate shorter string changed input")
			}

			return
		}

		if !strings.HasSuffix(got, "...") {
			t.Fatalf("expected ellipsis, got %q", got)
		}

		if len(got) != n+3 {
			t.Fatalf("len=%d want %d", len(got), n+3)
		}
	})
}

func TestTruncateTailRapid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "s")
		n := rapid.IntRange(1, 64).Draw(t, "n")
		got := truncateTail(s, n)

		trimmed := strings.TrimSpace(s)
		if len(trimmed) <= n {
			if got != trimmed {
				t.Fatalf("short tail truncate changed input")
			}

			return
		}

		if !strings.HasPrefix(got, "...\n") {
			t.Fatalf("expected prefix, got %q", got)
		}
	})
}

func TestSelectDiscoveryHarnessRapid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		secrets := map[string]string{}
		if rapid.Bool().Draw(t, "openai") {
			secrets[models.SecretNameOpenAIAPIKey] = "sk-test"
		}

		if rapid.Bool().Draw(t, "anthropic") {
			secrets[models.SecretNameAnthropicAPIKey] = "sk-ant"
		}

		if rapid.Bool().Draw(t, "openrouter") {
			secrets[models.SecretNameOpenRouterAPIKey] = "or-key"
		}

		bins := map[string]string{}

		for _, name := range []string{"codex", "claude", "omp", "opencode"} {
			if rapid.Bool().Draw(t, "bin-"+name) {
				bins[name] = "/usr/bin/" + name
			}
		}

		look := func(file string) (string, error) {
			if p, ok := bins[file]; ok {
				return p, nil
			}

			return "", fmt.Errorf("not found: %s", file)
		}

		h, err := SelectDiscoveryHarness(secrets, look)

		hasPair := (secrets[models.SecretNameOpenAIAPIKey] != "" && bins["codex"] != "") ||
			(secrets[models.SecretNameAnthropicAPIKey] != "" && bins["claude"] != "") ||
			(secrets[models.SecretNameOpenRouterAPIKey] != "" && bins["omp"] != "") ||
			(secrets[models.SecretNameOpenRouterAPIKey] != "" && bins["opencode"] != "")
		if hasPair {
			if err != nil {
				t.Fatalf("expected harness, got %v", err)
			}

			if h.BinaryPath == "" || h.Provider == "" {
				t.Fatalf("incomplete harness: %#v", h)
			}

			return
		}

		if err == nil {
			t.Fatal("expected ErrNoDiscoveryHarness")
		}
	})
}

func FuzzStripMarkdownFence(f *testing.F) {
	f.Add("```\nFROM alpine\n```")
	f.Add("plain text")
	f.Add("```dockerfile\nFROM x\n```")
	f.Fuzz(func(t *testing.T, text string) {
		_ = stripMarkdownFence(text)
	})
}
