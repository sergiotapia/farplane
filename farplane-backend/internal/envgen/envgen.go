// Package envgen builds a Project Environment Dockerfile from a repository checkout.
package envgen

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/lanetemplate"
)

const (
	// MaxGenerateAttempts is generate + repair turns (Go builds between turns).
	MaxGenerateAttempts = 5
	// AgentTurnTimeout caps each headless harness invocation.
	AgentTurnTimeout = 90 * time.Second
	// FarplaneBaseDockerfileName is written into the workspace for the agent to copy.
	FarplaneBaseDockerfileName = "farplane-base.Dockerfile"
	// BuildLogFeedbackBytes is the tail size fed back on repair turns.
	BuildLogFeedbackBytes = 4000
)

// Request is the input for Project Environment generation.
type Request struct {
	// WorkspaceDir is a local checkout of the GitHub repository (already cloned).
	WorkspaceDir string
	// RepoFullName is owner/name for prompt context.
	RepoFullName string
	// Secrets are decrypted organization secrets (API keys).
	Secrets map[string]string
}

// Result is the generated Dockerfile, discovery log, and optional built image.
type Result struct {
	DockerfileText string
	Log            string
	// ImageReference is set when Go's docker build succeeded during generation.
	ImageReference string
	// BuildLog is the last docker build log (success or final failure).
	BuildLog string
}

// Generator produces a Project Environment Dockerfile.
type Generator interface {
	Generate(ctx context.Context, req Request) (Result, error)
}

// ImageBuilder builds a Dockerfile the same way Farplane Validate does.
type ImageBuilder func(ctx context.Context, dockerfileText, tag string) (imageReference, logText string, err error)

// Service runs a host discovery harness against a cloned repository, then builds
// with ImageBuilder, repairing via the agent up to MaxGenerateAttempts times.
type Service struct {
	LookPath   LookPathFunc
	RunCommand RunCommandFunc
	BuildImage ImageBuilder
}

// New returns a Service with default PATH lookup.
func New() *Service {
	return &Service{
		LookPath: exec.LookPath,
	}
}

// Generate runs agent write → docker build → repair (max 5) until the image builds.
func (s *Service) Generate(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.WorkspaceDir) == "" {
		return Result{}, fmt.Errorf("workspace dir is required")
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
	if harnessErr != nil {
		return Result{}, fmt.Errorf("%w", harnessErr)
	}

	if err := MaterializeBuildContext(req.WorkspaceDir); err != nil {
		return Result{}, fmt.Errorf("materialize build context: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(req.WorkspaceDir, FarplaneBaseDockerfileName),
		[]byte(base),
		0o644,
	); err != nil {
		return Result{}, fmt.Errorf("write base dockerfile: %w", err)
	}

	if s.BuildImage == nil {
		return Result{}, fmt.Errorf("image builder is required")
	}

	repo := strings.TrimSpace(req.RepoFullName)
	if repo == "" {
		repo = "(unknown repo)"
	}
	log.Printf(
		"envgen start repo=%s workspace=%s provider=%s binary=%s model=%s max_attempts=%d agent_timeout=%s",
		repo, req.WorkspaceDir, harness.Provider, harness.BinaryName, harness.AgentModel,
		MaxGenerateAttempts, AgentTurnTimeout,
	)

	var genLog strings.Builder
	genLog.WriteString(fmt.Sprintf(
		"Discovery harness: provider=%s binary=%s model=%s\n",
		harness.Provider, harness.BinaryName, harness.AgentModel,
	))

	var lastDockerfile string
	var lastBuildLog string
	outPath := filepath.Join(req.WorkspaceDir, GeneratedDockerfileName)
	for attempt := 1; attempt <= MaxGenerateAttempts; attempt++ {
		var prompt string
		phase := "initial"
		if attempt == 1 {
			_ = os.Remove(outPath)
			prompt = buildInitialPrompt(req.RepoFullName, GeneratedDockerfileName, FarplaneBaseDockerfileName)
		} else {
			phase = "repair"
			// Leave the broken Dockerfile on disk so the agent can read and rewrite it.
			if err := os.WriteFile(outPath, []byte(lastDockerfile+"\n"), 0o644); err != nil {
				return Result{Log: genLog.String(), BuildLog: lastBuildLog}, fmt.Errorf(
					"write dockerfile for repair: %w", err,
				)
			}
			prompt = buildRepairPrompt(
				req.RepoFullName,
				GeneratedDockerfileName,
				truncateTail(lastBuildLog, BuildLogFeedbackBytes),
				attempt,
				MaxGenerateAttempts,
			)
		}

		log.Printf("envgen agent start repo=%s attempt=%d/%d phase=%s", repo, attempt, MaxGenerateAttempts, phase)
		genLog.WriteString(fmt.Sprintf("\n--- agent attempt %d/%d ---\n", attempt, MaxGenerateAttempts))
		agentStarted := time.Now()
		dockerfile, harnessLog, runErr := s.runDiscoveryHarness(ctx, harness, req, prompt)
		agentElapsed := time.Since(agentStarted).Round(time.Millisecond)
		genLog.WriteString(harnessLog)
		if runErr != nil || strings.TrimSpace(dockerfile) == "" {
			if runErr != nil {
				log.Printf(
					"envgen agent failed repo=%s attempt=%d/%d elapsed=%s err=%v",
					repo, attempt, MaxGenerateAttempts, agentElapsed, runErr,
				)
				genLog.WriteString(fmt.Sprintf("agent failed: %v\n", runErr))
			} else {
				log.Printf(
					"envgen agent empty dockerfile repo=%s attempt=%d/%d elapsed=%s",
					repo, attempt, MaxGenerateAttempts, agentElapsed,
				)
				genLog.WriteString("agent returned empty Dockerfile\n")
			}
			// Keep repairing while we still have a prior Dockerfile + build failure.
			if lastDockerfile == "" || strings.TrimSpace(lastBuildLog) == "" {
				if runErr != nil {
					return Result{Log: genLog.String(), BuildLog: lastBuildLog}, fmt.Errorf(
						"discovery harness failed on attempt %d: %w", attempt, runErr,
					)
				}
				return Result{Log: genLog.String(), BuildLog: lastBuildLog}, fmt.Errorf(
					"discovery harness returned an empty Dockerfile on attempt %d", attempt,
				)
			}
			log.Printf(
				"envgen agent retry repo=%s attempt=%d/%d keeping prior dockerfile",
				repo, attempt, MaxGenerateAttempts,
			)
			genLog.WriteString("keeping prior Dockerfile; will retry repair on next attempt\n")
			continue
		}
		lastDockerfile = strings.TrimSpace(dockerfile)
		log.Printf(
			"envgen agent ok repo=%s attempt=%d/%d elapsed=%s dockerfile_bytes=%d",
			repo, attempt, MaxGenerateAttempts, agentElapsed, len(lastDockerfile),
		)

		tag := fmt.Sprintf("farplane-envgen:%d-%d", time.Now().Unix(), attempt)
		log.Printf("envgen docker build start repo=%s attempt=%d/%d tag=%s", repo, attempt, MaxGenerateAttempts, tag)
		genLog.WriteString(fmt.Sprintf("\n--- docker build attempt %d/%d ---\n", attempt, MaxGenerateAttempts))
		buildStarted := time.Now()
		imageRef, buildLog, buildErr := s.BuildImage(ctx, lastDockerfile, tag)
		buildElapsed := time.Since(buildStarted).Round(time.Millisecond)
		lastBuildLog = buildLog
		if buildErr == nil {
			log.Printf(
				"envgen docker build ok repo=%s attempt=%d/%d elapsed=%s image=%s",
				repo, attempt, MaxGenerateAttempts, buildElapsed, imageRef,
			)
			genLog.WriteString("docker build succeeded\n")
			if strings.TrimSpace(buildLog) != "" {
				genLog.WriteString(truncateTail(buildLog, 2000))
				genLog.WriteString("\n")
			}
			log.Printf("envgen success repo=%s attempts=%d image=%s", repo, attempt, imageRef)
			return Result{
				DockerfileText: lastDockerfile,
				Log:            genLog.String(),
				ImageReference: imageRef,
				BuildLog:       buildLog,
			}, nil
		}
		log.Printf(
			"envgen docker build failed repo=%s attempt=%d/%d elapsed=%s err=%v tail=%q",
			repo, attempt, MaxGenerateAttempts, buildElapsed, buildErr,
			truncateTail(buildLog, 400),
		)
		genLog.WriteString("docker build failed: ")
		genLog.WriteString(buildErr.Error())
		genLog.WriteString("\n")
		genLog.WriteString(truncateTail(buildLog, 2000))
		genLog.WriteString("\n")
	}

	log.Printf("envgen failed repo=%s after %d build attempts", repo, MaxGenerateAttempts)
	return Result{
		DockerfileText: lastDockerfile,
		Log:            genLog.String(),
		BuildLog:       lastBuildLog,
	}, fmt.Errorf("environment generation failed after %d build attempts", MaxGenerateAttempts)
}

// MaterializeBuildContext writes Farplane bridge files into dest so
// docker build can satisfy COPY bridge (same as Validate).
func MaterializeBuildContext(dest string) error {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return fmt.Errorf("destination is required")
	}
	return fs.WalkDir(lanetemplate.BuildContextFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." || path == "Dockerfile" {
			return nil
		}
		target := filepath.Join(dest, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(lanetemplate.BuildContextFS(), path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
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

func truncateTail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "...\n" + s[len(s)-n:]
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
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w: %s", err, truncate(string(out), 800))
	}
	return dir, nil
}
