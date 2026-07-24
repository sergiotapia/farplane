// Package dockerlint validates Dockerfile text for Lane templates.
package dockerlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result is the outcome of linting a Dockerfile.
type Result struct {
	OK  bool
	Log string
}

var knownInstructions = map[string]struct{}{
	"FROM": {}, "RUN": {}, "CMD": {}, "LABEL": {}, "MAINTAINER": {},
	"EXPOSE": {}, "ENV": {}, "ADD": {}, "COPY": {}, "ENTRYPOINT": {},
	"VOLUME": {}, "USER": {}, "WORKDIR": {}, "ARG": {}, "ONBUILD": {},
	"STOPSIGNAL": {}, "HEALTHCHECK": {}, "SHELL": {},
}

// Lint checks Dockerfile text with a parse gate, then hadolint or docker build --check.
func Lint(ctx context.Context, dockerfileText string) Result {
	text := strings.TrimSpace(dockerfileText)
	if text == "" {
		return Result{OK: false, Log: "dockerfile is empty"}
	}

	if err := parseGate(text); err != nil {
		return Result{OK: false, Log: err.Error()}
	}

	if log, ok, ran := tryHadolint(ctx, text); ran {
		return Result{OK: ok, Log: log}
	}

	if log, ok, ran := tryDockerBuildCheck(ctx, text); ran {
		return Result{OK: ok, Log: log}
	}
	// Parse gate alone is enough when no external linter is available.
	return Result{OK: true, Log: "parse ok (no hadolint or docker build --check available)"}
}

func parseGate(text string) error { //nolint:gocyclo // multi-branch orchestration; keep under threshold when rewriting
	sawFrom := false
	continued := false

	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// Blank lines do not break a backslash continuation.
			continue
		}

		if strings.HasPrefix(trimmed, "#") && !continued {
			continue
		}
		// Dockerfile continuations end the previous physical line with `\`.
		if continued {
			continued = strings.HasSuffix(trimmed, "\\")
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}

		inst := strings.ToUpper(fields[0])
		if _, ok := knownInstructions[inst]; !ok {
			return fmt.Errorf("line %d: unknown Dockerfile instruction %q", i+1, fields[0])
		}

		if inst == "FROM" {
			sawFrom = true

			if len(fields) < 2 {
				return fmt.Errorf("line %d: FROM requires an image", i+1)
			}
		}

		continued = strings.HasSuffix(trimmed, "\\")
	}

	if !sawFrom {
		return errors.New("dockerfile must contain a FROM instruction")
	}

	return nil
}

func tryHadolint(ctx context.Context, text string) (log string, ok, ran bool) {
	path, err := exec.LookPath("hadolint")
	if err != nil {
		return "", false, false
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	//nolint:gosec // G204: command binary is a fixed tool name or resolved from PATH on purpose.
	cmd := exec.CommandContext(ctx, path, "-")
	cmd.Stdin = strings.NewReader(text)

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()

	log = strings.TrimSpace(out.String())
	if err != nil {
		if log == "" {
			log = err.Error()
		}

		return log, false, true
	}

	if log == "" {
		log = "hadolint ok"
	}

	return log, true, true
}

func tryDockerBuildCheck(ctx context.Context, text string) (log string, ok, ran bool) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", false, false
	}

	dir, err := os.MkdirTemp("", "farplane-dockerlint-*")
	if err != nil {
		return "", false, false
	}

	defer func() { _ = os.RemoveAll(dir) }()

	dockerfilePath := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(text+"\n"), 0o600); err != nil {
		return "", false, false
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	//nolint:gosec // G204: command binary is a fixed tool name or resolved from PATH on purpose.
	cmd := exec.CommandContext(ctx, "docker", "build", "--check", "-f", dockerfilePath, dir)

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()

	log = strings.TrimSpace(out.String())
	if err != nil {
		if log == "" {
			log = err.Error()
		}

		return log, false, true
	}

	if log == "" {
		log = "docker build --check ok"
	}

	return log, true, true
}
