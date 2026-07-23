// Package envgen builds a Project Environment Dockerfile from a repository checkout.
package envgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/lanetemplate"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// Request is the input for Project Environment generation.
type Request struct {
	// WorkspaceDir is a local checkout of the GitHub repository (already cloned).
	WorkspaceDir string
	// RepoFullName is owner/name for prompt context.
	RepoFullName string
	// Secrets are decrypted organization secrets (API keys).
	Secrets map[string]string
	// ModelSource is anthropic, openai, or openrouter when using an LLM.
	ModelSource string
	// AgentModel is the model id to call (optional; defaults per source).
	AgentModel string
}

// Result is the generated Dockerfile and a short discovery log.
type Result struct {
	DockerfileText string
	Log            string
}

// Generator produces a Project Environment Dockerfile.
type Generator interface {
	Generate(ctx context.Context, req Request) (Result, error)
}

// HTTPDoer is the subset of http.Client used for LLM calls.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Service explores a repo with a host discovery harness (or falls back).
type Service struct {
	HTTP       HTTPDoer
	LookPath   LookPathFunc
	RunCommand RunCommandFunc
}

// New returns a Service with a default HTTP client.
func New() *Service {
	return &Service{
		HTTP:     &http.Client{Timeout: 2 * time.Minute},
		LookPath: exec.LookPath,
	}
}

// Generate explores WorkspaceDir and returns a Dockerfile that includes the Farplane base.
// Preference: host discovery harness (secret + PATH) → HTTP LLM → heuristic.
func (s *Service) Generate(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.WorkspaceDir) == "" {
		return Result{}, fmt.Errorf("workspace dir is required")
	}
	signals, exploreLog, err := exploreWorkspace(req.WorkspaceDir)
	if err != nil {
		return Result{}, err
	}
	base, err := lanetemplate.DefaultDockerfile()
	if err != nil {
		return Result{}, fmt.Errorf("default dockerfile: %w", err)
	}

	lookPath := s.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	harness, harnessErr := SelectDiscoveryHarness(req.Secrets, lookPath)
	if harnessErr == nil {
		dockerfile, harnessLog, runErr := s.runDiscoveryHarness(ctx, harness, req, base, signals)
		exploreLog += "\n" + harnessLog
		if runErr == nil && strings.TrimSpace(dockerfile) != "" {
			return Result{
				DockerfileText: ensureFarplaneRuntimeBits(dockerfile, base),
				Log:            exploreLog,
			}, nil
		}
		if runErr != nil {
			exploreLog += "Harness generation failed: " + runErr.Error() + "\nFalling back.\n"
		}
	} else {
		exploreLog += "\nNo host discovery harness: " + harnessErr.Error() + "\n"
	}

	source := strings.TrimSpace(req.ModelSource)
	if source == "" {
		source = pickModelSource(req.Secrets)
	}
	model := strings.TrimSpace(req.AgentModel)
	if model == "" {
		model = defaultModelForSource(source)
	}

	if source != "" && model != "" && s.HTTP != nil {
		dockerfile, llmLog, llmErr := s.generateWithLLM(ctx, req, source, model, base, signals)
		if llmErr == nil && strings.TrimSpace(dockerfile) != "" {
			return Result{
				DockerfileText: ensureFarplaneRuntimeBits(dockerfile, base),
				Log:            exploreLog + "\n" + llmLog,
			}, nil
		}
		if llmErr != nil {
			exploreLog += "\nLLM generation failed: " + llmErr.Error() + "\nFalling back to heuristic Dockerfile.\n"
		}
	} else {
		exploreLog += "Using heuristic Dockerfile.\n"
	}

	return Result{
		DockerfileText: heuristicDockerfile(base, signals),
		Log:            exploreLog,
	}, nil
}

type repoSignals struct {
	HasPackageJSON        bool
	HasGoMod              bool
	HasGemfile            bool
	HasRequirements       bool
	HasPyproject          bool
	HasCargoToml          bool
	HasExistingDockerfile bool
	ExistingDockerfile    string
	TreeSummary           string
	KeyFileSnippets       string
}

func exploreWorkspace(root string) (repoSignals, string, error) {
	var sig repoSignals
	var log strings.Builder
	log.WriteString("Explored repository at " + root + "\n")

	entries := []struct {
		rel  string
		flag *bool
	}{
		{"package.json", &sig.HasPackageJSON},
		{"go.mod", &sig.HasGoMod},
		{"Gemfile", &sig.HasGemfile},
		{"requirements.txt", &sig.HasRequirements},
		{"pyproject.toml", &sig.HasPyproject},
		{"Cargo.toml", &sig.HasCargoToml},
		{"Dockerfile", &sig.HasExistingDockerfile},
	}
	for _, e := range entries {
		path := filepath.Join(root, e.rel)
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			*e.flag = true
			log.WriteString("found " + e.rel + "\n")
			if e.rel == "Dockerfile" {
				b, readErr := os.ReadFile(path)
				if readErr == nil {
					sig.ExistingDockerfile = string(b)
				}
			}
		}
	}

	var tree []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(rel)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if len(tree) < 80 {
			tree = append(tree, rel)
		}
		return nil
	})
	sig.TreeSummary = strings.Join(tree, "\n")

	var snippets strings.Builder
	for _, name := range []string{
		"package.json", "go.mod", "Gemfile", "requirements.txt", "pyproject.toml", "Cargo.toml", "README.md",
	} {
		path := filepath.Join(root, name)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(b)
		if len(text) > 4000 {
			text = text[:4000] + "\n...truncated...\n"
		}
		snippets.WriteString("===== " + name + " =====\n")
		snippets.WriteString(text)
		snippets.WriteString("\n\n")
	}
	sig.KeyFileSnippets = snippets.String()
	return sig, log.String(), nil
}

func heuristicDockerfile(base string, sig repoSignals) string {
	if sig.HasExistingDockerfile && strings.TrimSpace(sig.ExistingDockerfile) != "" {
		return ensureFarplaneRuntimeBits(sig.ExistingDockerfile, base)
	}
	var extra strings.Builder
	extra.WriteString("\n# Project Environment layers (heuristic)\n")
	if sig.HasGoMod {
		extra.WriteString("RUN apt-get update && apt-get install -y --no-install-recommends golang-go && rm -rf /var/lib/apt/lists/*\n")
	}
	if sig.HasPackageJSON {
		extra.WriteString("# Node.js is already installed in the Farplane base image.\n")
	}
	if sig.HasGemfile {
		extra.WriteString("# Ruby/Bundler are already installed in the Farplane base image.\n")
	}
	if sig.HasRequirements || sig.HasPyproject {
		extra.WriteString("# Python is already installed in the Farplane base image.\n")
	}
	if sig.HasCargoToml {
		extra.WriteString("RUN apt-get update && apt-get install -y --no-install-recommends curl build-essential && rm -rf /var/lib/apt/lists/* \\\n")
		extra.WriteString("    && curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y\n")
		extra.WriteString("ENV PATH=\"/root/.cargo/bin:${PATH}\"\n")
	}
	return appendBeforeWorkdir(base, extra.String())
}

func appendBeforeWorkdir(base, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	marker := "\nWORKDIR /workspace\n"
	if idx := strings.Index(base, marker); idx >= 0 {
		return base[:idx] + "\n" + extra + "\n" + base[idx:]
	}
	return base + "\n" + extra + "\n"
}

// ensureFarplaneRuntimeBits merges project Dockerfile with Farplane bridge/agent requirements.
func ensureFarplaneRuntimeBits(dockerfile, base string) string {
	text := strings.TrimSpace(dockerfile)
	if text == "" {
		return base
	}
	lower := strings.ToLower(text)
	needsBridge := !strings.Contains(lower, "/opt/farplane/bridge")
	needsAgents := !strings.Contains(lower, "claude-code") && !strings.Contains(lower, "@anthropic-ai/claude-code")
	if !needsBridge && !needsAgents {
		if !strings.Contains(lower, "expose 7420") {
			text += "\nEXPOSE 7420\n"
		}
		if !strings.Contains(lower, "bridge.js") {
			text += "\nENTRYPOINT [\"node\", \"/opt/farplane/bridge/bridge.js\"]\n"
		}
		return text
	}
	// Prefer Farplane base with a note that the model/heuristic suggested project layers.
	return appendBeforeWorkdir(base, "# Merged with Farplane agent runtime base.\n# Original suggestion retained as comments when needed for review.\n")
}

func pickModelSource(secrets map[string]string) string {
	if strings.TrimSpace(secrets[models.SecretNameAnthropicAPIKey]) != "" {
		return "anthropic"
	}
	if strings.TrimSpace(secrets[models.SecretNameOpenAIAPIKey]) != "" {
		return "openai"
	}
	if strings.TrimSpace(secrets[models.SecretNameOpenRouterAPIKey]) != "" {
		return "openrouter"
	}
	return ""
}

func defaultModelForSource(source string) string {
	switch source {
	case "anthropic":
		return "claude-sonnet-4-20250514"
	case "openai":
		return "gpt-4.1"
	case "openrouter":
		return "anthropic/claude-sonnet-4"
	default:
		return ""
	}
}

func (s *Service) generateWithLLM(
	ctx context.Context,
	req Request,
	source, model, base string,
	sig repoSignals,
) (string, string, error) {
	prompt := buildPrompt(req.RepoFullName, base, sig)
	switch source {
	case "anthropic":
		key := strings.TrimSpace(req.Secrets[models.SecretNameAnthropicAPIKey])
		if key == "" {
			return "", "", fmt.Errorf("ANTHROPIC_API_KEY is not set")
		}
		return s.callAnthropic(ctx, key, model, prompt)
	case "openai":
		key := strings.TrimSpace(req.Secrets[models.SecretNameOpenAIAPIKey])
		if key == "" {
			return "", "", fmt.Errorf("OPENAI_API_KEY is not set")
		}
		return s.callOpenAICompatible(ctx, "https://api.openai.com/v1/chat/completions", key, model, prompt)
	case "openrouter":
		key := strings.TrimSpace(req.Secrets[models.SecretNameOpenRouterAPIKey])
		if key == "" {
			return "", "", fmt.Errorf("OPENROUTER_API_KEY is not set")
		}
		return s.callOpenAICompatible(ctx, "https://openrouter.ai/api/v1/chat/completions", key, model, prompt)
	default:
		return "", "", fmt.Errorf("unsupported model source %q", source)
	}
}

func buildPrompt(repoFullName, base string, sig repoSignals) string {
	var b strings.Builder
	b.WriteString("You generate a Dockerfile for a Farplane Project Environment.\n")
	b.WriteString("Farplane runs AI coding agents inside this image with full permissions.\n")
	b.WriteString("The Dockerfile MUST keep the Farplane agent bridge and agent CLIs.\n")
	b.WriteString("Start from this Farplane base Dockerfile and add only what the repo needs ")
	b.WriteString("(languages, system packages, databases clients, build tools).\n")
	b.WriteString("Return ONLY the full Dockerfile text, no markdown fences.\n\n")
	b.WriteString("Repository: " + repoFullName + "\n\n")
	b.WriteString("===== Farplane base Dockerfile =====\n")
	b.WriteString(base)
	b.WriteString("\n\n===== File tree (sample) =====\n")
	b.WriteString(sig.TreeSummary)
	b.WriteString("\n\n===== Key files =====\n")
	b.WriteString(sig.KeyFileSnippets)
	if sig.HasExistingDockerfile {
		b.WriteString("\n===== Existing Dockerfile =====\n")
		b.WriteString(sig.ExistingDockerfile)
	}
	return b.String()
}

func (s *Service) callAnthropic(ctx context.Context, apiKey, model, prompt string) (string, string, error) {
	body := map[string]any{
		"model":      model,
		"max_tokens": 8192,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	resp, err := s.HTTP.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", err
	}
	var text strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	dockerfile := stripMarkdownFence(text.String())
	return dockerfile, "Generated Dockerfile via Anthropic model " + model + "\n", nil
}

func (s *Service) callOpenAICompatible(ctx context.Context, url, apiKey, model, prompt string) (string, string, error) {
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := s.HTTP.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("llm HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", err
	}
	if len(parsed.Choices) == 0 {
		return "", "", fmt.Errorf("llm returned no choices")
	}
	dockerfile := stripMarkdownFence(parsed.Choices[0].Message.Content)
	return dockerfile, "Generated Dockerfile via model " + model + "\n", nil
}

func stripMarkdownFence(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			text = strings.Join(lines, "\n")
		}
	}
	return strings.TrimSpace(text)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// CloneRepository clones a GitHub repo into a temp directory using an installation token.
func CloneRepository(ctx context.Context, fullName, branch, token string) (string, error) {
	fullName = strings.TrimSpace(fullName)
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	if fullName == "" || token == "" {
		return "", fmt.Errorf("fullName and token are required")
	}
	dir, err := os.MkdirTemp("", "farplane-envgen-*")
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, fullName)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", branch, url, dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w: %s", err, truncate(string(out), 800))
	}
	return dir, nil
}
