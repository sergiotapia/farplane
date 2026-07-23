package envgen_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/envgen"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

func TestSelectDiscoveryHarnessPrefersOhMyPiWithOpenRouter(t *testing.T) {
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
}

func TestHarnessArgsOhMyPi(t *testing.T) {
	args, err := envgen.HarnessArgs(models.AgentProviderOhMyPi, "prompt here")
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
}

func TestGenerateUsesDiscoveryHarness(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dockerfile := "FROM debian:bookworm-slim\nWORKDIR /workspace\nCOPY bridge /opt/farplane/bridge\nEXPOSE 7420\nENTRYPOINT [\"node\", \"/opt/farplane/bridge/bridge.js\"]\n"
	svc := envgen.New()
	svc.HTTP = nil
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
	if !strings.Contains(result.Log, "provider=oh_my_pi") {
		t.Fatalf("log = %q", result.Log)
	}
}
