package envgen_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/envgen"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

func TestSelectDiscoveryHarnessPrefersCodex(t *testing.T) {
	secrets := map[string]string{
		models.SecretNameOpenAIAPIKey:     "sk-openai",
		models.SecretNameAnthropicAPIKey:  "sk-ant",
		models.SecretNameOpenRouterAPIKey: "sk-or-test",
	}
	look := func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}

	h, err := envgen.SelectDiscoveryHarness(secrets, look)
	if err != nil {
		t.Fatalf("SelectDiscoveryHarness: %v", err)
	}

	if h.Provider != models.AgentProviderCodex {
		t.Fatalf("provider = %q, want codex", h.Provider)
	}

	if h.AgentModel != envgen.DiscoveryModelCodex {
		t.Fatalf("model = %q, want %q", h.AgentModel, envgen.DiscoveryModelCodex)
	}
}

func TestSelectDiscoveryHarnessPrefersOhMyPiWithOpenRouterOnly(t *testing.T) {
	secrets := map[string]string{
		models.SecretNameOpenRouterAPIKey: "sk-or-test",
	}
	look := func(file string) (string, error) {
		switch file {
		case "omp":
			return "/usr/local/bin/omp", nil
		case "opencode":
			return "/usr/local/bin/opencode", nil
		default:
			return "", errors.New("not found")
		}
	}

	h, err := envgen.SelectDiscoveryHarness(secrets, look)
	if err != nil {
		t.Fatalf("SelectDiscoveryHarness: %v", err)
	}

	if h.Provider != models.AgentProviderOhMyPi {
		t.Fatalf("provider = %q, want oh_my_pi", h.Provider)
	}

	if h.BinaryName != "omp" || h.BinaryPath != "/usr/local/bin/omp" {
		t.Fatalf("binary = %#v", h)
	}

	if h.ModelSource != "openrouter" {
		t.Fatalf("model_source = %q", h.ModelSource)
	}

	if h.SecretName != models.SecretNameOpenRouterAPIKey {
		t.Fatalf("secret = %q", h.SecretName)
	}

	if h.AgentModel != envgen.DiscoveryModelOpenRouter {
		t.Fatalf("model = %q, want %q", h.AgentModel, envgen.DiscoveryModelOpenRouter)
	}
}

func TestSelectDiscoveryHarnessRequiresSecretAndBinary(t *testing.T) {
	_, err := envgen.SelectDiscoveryHarness(map[string]string{
		models.SecretNameOpenRouterAPIKey: "sk-or-test",
	}, func(file string) (string, error) {
		return "", errors.New("missing")
	})
	if !errors.Is(err, envgen.ErrNoDiscoveryHarness) {
		t.Fatalf("err = %v, want ErrNoDiscoveryHarness", err)
	}

	_, err = envgen.SelectDiscoveryHarness(nil, func(file string) (string, error) {
		return "/bin/" + file, nil
	})
	if !errors.Is(err, envgen.ErrNoDiscoveryHarness) {
		t.Fatalf("err = %v, want ErrNoDiscoveryHarness", err)
	}
}

func TestSelectDiscoveryHarnessFallsBackToClaude(t *testing.T) {
	secrets := map[string]string{
		models.SecretNameAnthropicAPIKey: "sk-ant",
	}

	h, err := envgen.SelectDiscoveryHarness(secrets, func(file string) (string, error) {
		if file == "claude" {
			return "/bin/claude", nil
		}

		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("SelectDiscoveryHarness: %v", err)
	}

	if h.Provider != models.AgentProviderClaudeCode {
		t.Fatalf("provider = %q", h.Provider)
	}

	if h.AgentModel != envgen.DiscoveryModelClaude {
		t.Fatalf("model = %q, want %q", h.AgentModel, envgen.DiscoveryModelClaude)
	}
}

func TestSelectDiscoveryHarnessFallsBackToOpenCode(t *testing.T) {
	secrets := map[string]string{
		models.SecretNameOpenRouterAPIKey: "sk-or-test",
	}

	h, err := envgen.SelectDiscoveryHarness(secrets, func(file string) (string, error) {
		if file == "opencode" {
			return "/bin/opencode", nil
		}

		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("SelectDiscoveryHarness: %v", err)
	}

	if h.Provider != models.AgentProviderOpenCode {
		t.Fatalf("provider = %q", h.Provider)
	}

	if h.AgentModel != envgen.DiscoveryModelOpenRouter {
		t.Fatalf("model = %q", h.AgentModel)
	}
}

func TestHarnessArgsOhMyPiIncludesModelAndThinkingOff(t *testing.T) {
	args, err := envgen.HarnessArgs(
		models.AgentProviderOhMyPi, "prompt here", envgen.DiscoveryModelOpenRouter,
	)
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-p") || !strings.Contains(joined, "prompt here") {
		t.Fatalf("args = %#v", args)
	}

	if !strings.Contains(joined, "--approval-mode=yolo") {
		t.Fatalf("missing yolo: %#v", args)
	}

	if !strings.Contains(joined, "--thinking=off") {
		t.Fatalf("missing thinking=off: %#v", args)
	}

	if !strings.Contains(joined, "--model="+envgen.DiscoveryModelOpenRouter) {
		t.Fatalf("missing model: %#v", args)
	}
}

func TestGenerateUsesDiscoveryHarnessAndBuilds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dockerfile := "FROM debian:bookworm-slim\nWORKDIR /workspace\nCOPY bridge /opt/farplane/bridge\nEXPOSE 7420\nENTRYPOINT [\"node\", \"/opt/farplane/bridge/bridge.js\"]\n"
	svc := envgen.New()
	svc.LookPath = func(file string) (string, error) {
		if file == "omp" {
			return "/fake/omp", nil
		}

		return "", errors.New("missing")
	}
	svc.RunCommand = func(ctx context.Context, name string, args []string, workDir string, env []string) (string, error) {
		_ = ctx

		if name != "/fake/omp" {
			return "", fmt.Errorf("unexpected binary %s", name)
		}

		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--model="+envgen.DiscoveryModelOpenRouter) {
			return "", fmt.Errorf("missing model in args: %#v", args)
		}

		if !strings.Contains(joined, "Do NOT run docker build") {
			return "", fmt.Errorf("prompt must forbid in-agent docker build: %#v", args)
		}

		if strings.Contains(joined, "docker build -t") {
			return "", fmt.Errorf("prompt must not ask agent to docker build: %#v", args)
		}

		if !strings.Contains(joined, envgen.FarplaneBaseDockerfileName) {
			return "", fmt.Errorf("prompt missing base dockerfile name: %#v", args)
		}

		if strings.Contains(joined, "===== File tree") || strings.Contains(joined, "===== Key files") {
			return "", fmt.Errorf("prompt still embeds pre-scanned repo signals: %#v", args)
		}

		if _, err := os.Stat(filepath.Join(workDir, "bridge")); err != nil {
			return "", fmt.Errorf("bridge build context missing: %w", err)
		}

		if _, err := os.Stat(filepath.Join(workDir, envgen.FarplaneBaseDockerfileName)); err != nil {
			return "", fmt.Errorf("base dockerfile missing: %w", err)
		}

		hasKey := false

		for _, e := range env {
			if strings.HasPrefix(e, models.SecretNameOpenRouterAPIKey+"=") {
				hasKey = true
			}
		}

		if !hasKey {
			return "", errors.New("OPENROUTER_API_KEY not injected")
		}

		out := filepath.Join(workDir, envgen.GeneratedDockerfileName)
		if err := os.WriteFile(out, []byte(dockerfile), 0o644); err != nil {
			return "", err
		}

		return "ok", nil
	}
	svc.BuildImage = func(ctx context.Context, dockerfileText, tag string) (string, string, error) {
		_ = ctx
		_ = tag

		if !strings.Contains(dockerfileText, "FROM debian:bookworm-slim") {
			return "", "", errors.New("unexpected dockerfile")
		}

		return "farplane-envgen:ok", "build ok", nil
	}

	result, err := svc.Generate(context.Background(), envgen.Request{
		WorkspaceDir: dir,
		RepoFullName: "alice/app",
		Secrets: map[string]string{
			models.SecretNameOpenRouterAPIKey: "sk-or-test",
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(result.DockerfileText, "FROM debian:bookworm-slim") {
		t.Fatalf("dockerfile = %s", result.DockerfileText)
	}

	if result.ImageReference != "farplane-envgen:ok" {
		t.Fatalf("image = %q", result.ImageReference)
	}

	if !strings.Contains(result.Log, "provider=oh_my_pi") {
		t.Fatalf("log = %q", result.Log)
	}

	if !strings.Contains(result.Log, "docker build succeeded") {
		t.Fatalf("log missing build success: %q", result.Log)
	}
}

func TestGenerateRetriesBuildFailuresThenSucceeds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dockerfile := "FROM debian:bookworm-slim\nCOPY bridge /opt/farplane/bridge\n"

	var agentCalls, buildCalls int

	svc := envgen.New()
	svc.LookPath = func(file string) (string, error) {
		if file == "omp" {
			return "/fake/omp", nil
		}

		return "", errors.New("missing")
	}
	svc.RunCommand = func(ctx context.Context, name string, args []string, workDir string, env []string) (string, error) {
		_ = ctx
		_ = name
		_ = env
		agentCalls++

		joined := strings.Join(args, " ")
		if agentCalls == 1 {
			if strings.Contains(joined, "repair attempt") {
				return "", errors.New("first turn should be initial prompt")
			}
		} else {
			if !strings.Contains(joined, "repair attempt") {
				return "", fmt.Errorf("attempt %d should be repair: %#v", agentCalls, args)
			}

			if !strings.Contains(joined, "build failed "+strconv.Itoa(agentCalls-1)) {
				return "", fmt.Errorf("repair missing prior build log: %#v", args)
			}
		}

		out := filepath.Join(workDir, envgen.GeneratedDockerfileName)
		if err := os.WriteFile(out, []byte(dockerfile+fmt.Sprintf("# attempt %d\n", agentCalls)), 0o644); err != nil {
			return "", err
		}

		return "ok", nil
	}
	svc.BuildImage = func(ctx context.Context, dockerfileText, tag string) (string, string, error) {
		_ = ctx
		_ = dockerfileText
		_ = tag

		buildCalls++
		if buildCalls < 3 {
			return "", fmt.Sprintf("build failed %d", buildCalls), errors.New("docker build failed")
		}

		return "farplane-envgen:third", "build ok", nil
	}

	result, err := svc.Generate(context.Background(), envgen.Request{
		WorkspaceDir: dir,
		RepoFullName: "alice/app",
		Secrets: map[string]string{
			models.SecretNameOpenRouterAPIKey: "sk-or-test",
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if agentCalls != 3 || buildCalls != 3 {
		t.Fatalf("agentCalls=%d buildCalls=%d, want 3/3", agentCalls, buildCalls)
	}

	if result.ImageReference != "farplane-envgen:third" {
		t.Fatalf("image = %q", result.ImageReference)
	}

	if !strings.Contains(result.DockerfileText, "# attempt 3") {
		t.Fatalf("dockerfile = %s", result.DockerfileText)
	}
}

func TestGenerateContinuesAfterAgentTimeoutOnRepair(t *testing.T) {
	dir := t.TempDir()
	dockerfile := "FROM debian:bookworm-slim\nCOPY bridge /opt/farplane/bridge\n"

	var agentCalls, buildCalls int

	svc := envgen.New()
	svc.LookPath = func(file string) (string, error) {
		if file == "omp" {
			return "/fake/omp", nil
		}

		return "", errors.New("missing")
	}
	svc.RunCommand = func(ctx context.Context, name string, args []string, workDir string, env []string) (string, error) {
		_ = ctx
		_ = name
		_ = env

		agentCalls++
		if agentCalls == 2 {
			return "", errors.New("signal: killed")
		}

		out := filepath.Join(workDir, envgen.GeneratedDockerfileName)
		if agentCalls >= 3 {
			// Repair should see the prior Dockerfile on disk.
			if _, err := os.Stat(out); err != nil {
				return "", fmt.Errorf("prior dockerfile missing on repair: %w", err)
			}
		}

		if err := os.WriteFile(out, []byte(dockerfile+fmt.Sprintf("# attempt %d\n", agentCalls)), 0o644); err != nil {
			return "", err
		}

		return "ok", nil
	}
	svc.BuildImage = func(ctx context.Context, dockerfileText, tag string) (string, string, error) {
		_ = ctx
		_ = tag

		buildCalls++
		if buildCalls == 1 {
			return "", "build failed 1", errors.New("docker build failed")
		}

		if !strings.Contains(dockerfileText, "# attempt 3") {
			return "", "", fmt.Errorf("unexpected dockerfile after timeout: %s", dockerfileText)
		}

		return "farplane-envgen:recovered", "build ok", nil
	}

	result, err := svc.Generate(context.Background(), envgen.Request{
		WorkspaceDir: dir,
		RepoFullName: "alice/app",
		Secrets: map[string]string{
			models.SecretNameOpenRouterAPIKey: "sk-or-test",
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if agentCalls != 3 || buildCalls != 2 {
		t.Fatalf("agentCalls=%d buildCalls=%d, want 3/2", agentCalls, buildCalls)
	}

	if result.ImageReference != "farplane-envgen:recovered" {
		t.Fatalf("image = %q", result.ImageReference)
	}
}

func TestGenerateStopsAfterMaxBuildAttempts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var agentCalls, buildCalls int

	svc := envgen.New()
	svc.LookPath = func(file string) (string, error) {
		if file == "omp" {
			return "/fake/omp", nil
		}

		return "", errors.New("missing")
	}
	svc.RunCommand = func(ctx context.Context, name string, args []string, workDir string, env []string) (string, error) {
		_ = ctx
		_ = name
		_ = args
		_ = env
		agentCalls++
		out := filepath.Join(workDir, envgen.GeneratedDockerfileName)

		return "", os.WriteFile(out, []byte("FROM debian:bookworm-slim\n"), 0o644)
	}
	svc.BuildImage = func(ctx context.Context, dockerfileText, tag string) (string, string, error) {
		_ = ctx
		_ = dockerfileText
		_ = tag
		buildCalls++

		return "", "always fails", errors.New("docker build failed")
	}

	_, err := svc.Generate(context.Background(), envgen.Request{
		WorkspaceDir: dir,
		RepoFullName: "alice/app",
		Secrets: map[string]string{
			models.SecretNameOpenRouterAPIKey: "sk-or-test",
		},
	})
	if err == nil {
		t.Fatal("expected error after max attempts")
	}

	if !strings.Contains(err.Error(), "after 5 build attempts") {
		t.Fatalf("err = %v", err)
	}

	if agentCalls != envgen.MaxGenerateAttempts || buildCalls != envgen.MaxGenerateAttempts {
		t.Fatalf("agentCalls=%d buildCalls=%d, want %d", agentCalls, buildCalls, envgen.MaxGenerateAttempts)
	}
}

func TestGenerateRequiresDiscoveryHarness(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := envgen.New()
	svc.LookPath = func(file string) (string, error) {
		return "", errors.New("not on PATH")
	}
	svc.BuildImage = func(ctx context.Context, dockerfileText, tag string) (string, string, error) {
		return "", "", errors.New("should not build")
	}

	_, err := svc.Generate(context.Background(), envgen.Request{
		WorkspaceDir: dir,
		RepoFullName: "alice/app",
		Secrets: map[string]string{
			models.SecretNameOpenRouterAPIKey: "sk-or-test",
		},
	})
	if !errors.Is(err, envgen.ErrNoDiscoveryHarness) {
		t.Fatalf("err = %v, want ErrNoDiscoveryHarness", err)
	}
}

func TestGenerateRequiresImageBuilder(t *testing.T) {
	dir := t.TempDir()
	svc := envgen.New()
	svc.LookPath = func(file string) (string, error) {
		if file == "omp" {
			return "/fake/omp", nil
		}

		return "", errors.New("missing")
	}

	_, err := svc.Generate(context.Background(), envgen.Request{
		WorkspaceDir: dir,
		Secrets: map[string]string{
			models.SecretNameOpenRouterAPIKey: "sk-or-test",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "image builder is required") {
		t.Fatalf("err = %v", err)
	}
}
