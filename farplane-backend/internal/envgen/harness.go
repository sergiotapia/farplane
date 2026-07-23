package envgen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// GeneratedDockerfileName is the file a discovery harness must write in the workspace.
const GeneratedDockerfileName = ".farplane-environment.Dockerfile"

// Fixed models for Project Environment discovery (host CLIs only).
const (
	DiscoveryModelCodex  = "gpt-5.1-codex"
	DiscoveryModelClaude = "opus"
	// Provider-prefixed so omp/opencode route via OpenRouter (not DeepSeek direct).
	DiscoveryModelOpenRouter = "openrouter/deepseek/deepseek-v4-flash"
)

// DiscoveryHarness is a host-installed agent CLI chosen for Project Environment generation.
type DiscoveryHarness struct {
	Provider    string
	BinaryName  string
	BinaryPath  string
	ModelSource string
	SecretName  string
	AgentModel  string
}

// LookPathFunc locates an executable on PATH (tests inject a fake).
type LookPathFunc func(file string) (string, error)

// RunCommandFunc runs a host command (tests inject a fake).
type RunCommandFunc func(ctx context.Context, name string, args []string, dir string, env []string) (string, error)

type harnessCandidate struct {
	Provider    string
	BinaryName  string
	ModelSource string
	SecretName  string
	AgentModel  string
}

// discoveryPreference is the order used when several harnesses are available.
var discoveryPreference = []harnessCandidate{
	{
		Provider:    models.AgentProviderCodex,
		BinaryName:  "codex",
		ModelSource: agents.ModelSourceOpenAI,
		SecretName:  models.SecretNameOpenAIAPIKey,
		AgentModel:  DiscoveryModelCodex,
	},
	{
		Provider:    models.AgentProviderClaudeCode,
		BinaryName:  "claude",
		ModelSource: agents.ModelSourceAnthropic,
		SecretName:  models.SecretNameAnthropicAPIKey,
		AgentModel:  DiscoveryModelClaude,
	},
	{
		Provider:    models.AgentProviderOhMyPi,
		BinaryName:  "omp",
		ModelSource: agents.ModelSourceOpenRouter,
		SecretName:  models.SecretNameOpenRouterAPIKey,
		AgentModel:  DiscoveryModelOpenRouter,
	},
	{
		Provider:    models.AgentProviderOpenCode,
		BinaryName:  "opencode",
		ModelSource: agents.ModelSourceOpenRouter,
		SecretName:  models.SecretNameOpenRouterAPIKey,
		AgentModel:  DiscoveryModelOpenRouter,
	},
}

// ErrNoDiscoveryHarness means no secret+binary pair can run discovery.
var ErrNoDiscoveryHarness = fmt.Errorf("no discovery harness available")

// SelectDiscoveryHarness picks a headless agent from org secrets and host PATH.
// Priority: codex → claude → oh-my-pi → opencode. Oh-my-pi and OpenCode require
// OPENROUTER_API_KEY and use DeepSeek V4 Flash.
func SelectDiscoveryHarness(secrets map[string]string, lookPath LookPathFunc) (DiscoveryHarness, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	var missing []string
	for _, candidate := range discoveryPreference {
		if strings.TrimSpace(secrets[candidate.SecretName]) == "" {
			missing = append(missing, candidate.Provider+": "+candidate.SecretName+" not set")
			continue
		}
		path, err := lookPath(candidate.BinaryName)
		if err != nil || strings.TrimSpace(path) == "" {
			missing = append(missing, candidate.Provider+": "+candidate.BinaryName+" not on PATH")
			continue
		}
		return DiscoveryHarness{
			Provider:    candidate.Provider,
			BinaryName:  candidate.BinaryName,
			BinaryPath:  path,
			ModelSource: candidate.ModelSource,
			SecretName:  candidate.SecretName,
			AgentModel:  candidate.AgentModel,
		}, nil
	}
	return DiscoveryHarness{}, fmt.Errorf("%w (%s)", ErrNoDiscoveryHarness, strings.Join(missing, "; "))
}

// HarnessArgs builds headless CLI args for a discovery harness (mirrors lane bridge).
func HarnessArgs(provider, prompt, model string) ([]string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("discovery model is required")
	}
	switch strings.TrimSpace(provider) {
	case models.AgentProviderOhMyPi:
		return []string{
			"-p",
			"--mode=json",
			"--auto-approve",
			"--approval-mode=yolo",
			"--no-session",
			"--thinking=off",
			"--model=" + model,
			prompt,
		}, nil
	case models.AgentProviderOpenCode:
		return []string{
			"run",
			"--auto",
			"--dangerously-skip-permissions",
			"--format",
			"json",
			"--model",
			model,
			prompt,
		}, nil
	case models.AgentProviderClaudeCode:
		return []string{
			"--bare",
			"--dangerously-skip-permissions",
			"--permission-mode",
			"bypassPermissions",
			"--model",
			model,
			"-p",
			prompt,
		}, nil
	case models.AgentProviderCodex:
		return []string{
			"--dangerously-bypass-approvals-and-sandbox",
			"exec",
			"--json",
			"-m",
			model,
			prompt,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported discovery harness %q", provider)
	}
}

func (s *Service) runDiscoveryHarness(
	ctx context.Context,
	harness DiscoveryHarness,
	req Request,
	prompt string,
) (string, string, error) {
	outPath := filepath.Join(req.WorkspaceDir, GeneratedDockerfileName)
	// Caller owns whether the file exists (initial clears; repair seeds the prior draft).

	args, err := HarnessArgs(harness.Provider, prompt, harness.AgentModel)
	if err != nil {
		return "", "", err
	}

	env := append([]string{}, os.Environ()...)
	for name, value := range req.Secrets {
		if strings.TrimSpace(value) == "" {
			continue
		}
		env = append(env, name+"="+value)
	}
	// OpenCode reads OPENCODE_*; keep YOLO for discovery.
	env = append(env,
		"OPENCODE_DANGEROUSLY_SKIP_PERMISSIONS=true",
		"OPENCODE_YOLO=true",
	)

	run := s.RunCommand
	if run == nil {
		run = defaultRunCommand
	}
	runCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, AgentTurnTimeout)
		defer cancel()
	}

	combined, runErr := run(runCtx, harness.BinaryPath, args, req.WorkspaceDir, env)
	logText := fmt.Sprintf(
		"Discovery harness: provider=%s binary=%s model_source=%s secret=%s model=%s\n",
		harness.Provider, harness.BinaryName, harness.ModelSource, harness.SecretName, harness.AgentModel,
	)
	if strings.TrimSpace(combined) != "" {
		logText += "Harness output (tail):\n" + truncate(combined, 4000) + "\n"
	}
	if runErr != nil {
		return "", logText, fmt.Errorf("harness %s: %w", harness.BinaryName, runErr)
	}

	raw, readErr := os.ReadFile(outPath)
	if readErr != nil {
		// Some harnesses may print the Dockerfile instead of writing the file.
		if extracted := stripMarkdownFence(combined); strings.Contains(extracted, "FROM ") {
			return extracted, logText + "Read Dockerfile from harness stdout (file missing).\n", nil
		}
		return "", logText, fmt.Errorf("harness did not write %s: %w", GeneratedDockerfileName, readErr)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", logText, fmt.Errorf("harness wrote empty %s", GeneratedDockerfileName)
	}
	return text, logText + "Wrote " + GeneratedDockerfileName + "\n", nil
}

func defaultRunCommand(ctx context.Context, name string, args []string, dir string, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
