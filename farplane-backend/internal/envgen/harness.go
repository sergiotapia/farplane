package envgen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// GeneratedDockerfileName is the file a discovery harness must write in the workspace.
const GeneratedDockerfileName = ".farplane-environment.Dockerfile"

// DiscoveryHarness is a host-installed agent CLI chosen for Project Environment generation.
type DiscoveryHarness struct {
	Provider    string
	BinaryName  string
	BinaryPath  string
	ModelSource string
	SecretName  string
}

// LookPathFunc locates an executable on PATH (tests inject a fake).
type LookPathFunc func(file string) (string, error)

// RunCommandFunc runs a host command (tests inject a fake).
type RunCommandFunc func(ctx context.Context, name string, args []string, dir string, env []string) (string, error)

type harnessCandidate struct {
	Provider   string
	BinaryName string
}

// discoveryPreference is the order used when several harnesses are available.
// Prefer oh-my-pi / OpenCode (multi-provider) before single-vendor CLIs.
var discoveryPreference = []harnessCandidate{
	{Provider: models.AgentProviderOhMyPi, BinaryName: "omp"},
	{Provider: models.AgentProviderOpenCode, BinaryName: "opencode"},
	{Provider: models.AgentProviderClaudeCode, BinaryName: "claude"},
	{Provider: models.AgentProviderCodex, BinaryName: "codex"},
}

// ErrNoDiscoveryHarness means no secret+binary pair can run discovery.
var ErrNoDiscoveryHarness = fmt.Errorf("no discovery harness available")

// SelectDiscoveryHarness picks a headless agent from org secrets and host PATH.
// A harness is eligible only when at least one of its model-source secrets is set
// and its CLI binary is on PATH (or will be installed on the Farplane host).
func SelectDiscoveryHarness(secrets map[string]string, lookPath LookPathFunc) (DiscoveryHarness, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	setSecrets := secretPresence(secrets)
	var missing []string
	for _, candidate := range discoveryPreference {
		if !agents.AgentAvailable(candidate.Provider, setSecrets) {
			missing = append(missing, candidate.Provider+": secret not set")
			continue
		}
		path, err := lookPath(candidate.BinaryName)
		if err != nil || strings.TrimSpace(path) == "" {
			missing = append(missing, candidate.Provider+": "+candidate.BinaryName+" not on PATH")
			continue
		}
		source, ok := agents.DefaultModelSource(candidate.Provider, setSecrets)
		if !ok {
			missing = append(missing, candidate.Provider+": no model source")
			continue
		}
		secretName := secretNameForModelSource(source)
		return DiscoveryHarness{
			Provider:    candidate.Provider,
			BinaryName:  candidate.BinaryName,
			BinaryPath:  path,
			ModelSource: source,
			SecretName:  secretName,
		}, nil
	}
	return DiscoveryHarness{}, fmt.Errorf("%w (%s)", ErrNoDiscoveryHarness, strings.Join(missing, "; "))
}

func secretPresence(secrets map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, name := range agents.WellKnownSecretNames {
		if strings.TrimSpace(secrets[name]) != "" {
			out[name] = true
		}
	}
	return out
}

func secretNameForModelSource(source string) string {
	switch strings.TrimSpace(source) {
	case agents.ModelSourceAnthropic:
		return models.SecretNameAnthropicAPIKey
	case agents.ModelSourceOpenAI:
		return models.SecretNameOpenAIAPIKey
	case agents.ModelSourceOpenRouter:
		return models.SecretNameOpenRouterAPIKey
	default:
		return ""
	}
}

// HarnessArgs builds headless CLI args for a discovery harness (mirrors lane bridge).
func HarnessArgs(provider, prompt string) ([]string, error) {
	switch strings.TrimSpace(provider) {
	case models.AgentProviderOhMyPi:
		return []string{
			"-p",
			"--mode=json",
			"--auto-approve",
			"--approval-mode=yolo",
			"--no-session",
			prompt,
		}, nil
	case models.AgentProviderOpenCode:
		return []string{
			"run",
			"--auto",
			"--dangerously-skip-permissions",
			"--format",
			"json",
			prompt,
		}, nil
	case models.AgentProviderClaudeCode:
		return []string{
			"--bare",
			"--dangerously-skip-permissions",
			"--permission-mode",
			"bypassPermissions",
			"-p",
			prompt,
		}, nil
	case models.AgentProviderCodex:
		return []string{
			"--dangerously-bypass-approvals-and-sandbox",
			"exec",
			"--json",
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
	base string,
	sig repoSignals,
) (string, string, error) {
	outPath := filepath.Join(req.WorkspaceDir, GeneratedDockerfileName)
	_ = os.Remove(outPath)

	prompt := buildHarnessPrompt(req.RepoFullName, base, sig, GeneratedDockerfileName)
	args, err := HarnessArgs(harness.Provider, prompt)
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
		runCtx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	combined, runErr := run(runCtx, harness.BinaryPath, args, req.WorkspaceDir, env)
	logText := fmt.Sprintf(
		"Discovery harness: provider=%s binary=%s model_source=%s secret=%s\n",
		harness.Provider, harness.BinaryName, harness.ModelSource, harness.SecretName,
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

func buildHarnessPrompt(repoFullName, base string, sig repoSignals, outFile string) string {
	var b strings.Builder
	b.WriteString("You are exploring a Git repository checkout to produce a Farplane Project Environment Dockerfile.\n")
	b.WriteString("Farplane runs AI coding agents inside this image with full sandbox permissions.\n")
	b.WriteString("Explore the repo (languages, package managers, databases, build tools).\n")
	b.WriteString("Write ONE complete Dockerfile to the file exactly named: ")
	b.WriteString(outFile)
	b.WriteString("\n")
	b.WriteString("Start from the Farplane base Dockerfile below and add only what this repo needs.\n")
	b.WriteString("Keep the Farplane agent bridge, agent CLIs, EXPOSE 7420, and bridge ENTRYPOINT.\n")
	b.WriteString("Do not ask questions. When finished, the file must exist and contain only Dockerfile text.\n\n")
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
