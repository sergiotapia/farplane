package envgen

import (
	"strings"
	"testing"
)

func TestBuildInitialPromptOmitsInAgentDockerBuild(t *testing.T) {
	prompt := buildInitialPrompt("alice/app", GeneratedDockerfileName, FarplaneBaseDockerfileName)
	for _, want := range []string{
		"Farplane",
		"Lane",
		"Project Environment",
		GeneratedDockerfileName,
		FarplaneBaseDockerfileName,
		"Do NOT run docker build",
		"At most 5 tool calls",
		"Prefer mise for ALL language/runtime installs",
		"erlang@27.2 then elixir@1.18.3",
		"mix archive.install github hexpm/hex",
		"alice/app",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("initial prompt missing %q:\n%s", want, prompt)
		}
	}

	if strings.Contains(prompt, "docker build -t") {
		t.Fatalf("initial prompt must not ask the agent to docker build:\n%s", prompt)
	}

	if strings.Contains(prompt, "===== Farplane base Dockerfile =====") {
		t.Fatalf("initial prompt must not paste the full base Dockerfile:\n%s", prompt)
	}
}

func TestBuildRepairPromptIncludesBuildLog(t *testing.T) {
	prompt := buildRepairPrompt(
		"alice/app",
		GeneratedDockerfileName,
		"E: Unable to locate package postgresql-16",
		3,
		MaxGenerateAttempts,
	)
	for _, want := range []string{
		"repair attempt 3 of 5",
		"Unable to locate package postgresql-16",
		"Do NOT run docker build",
		"Read the current " + GeneratedDockerfileName,
		"Prefer mise for ALL language/runtime installs",
		"erlang BEFORE elixir",
		GeneratedDockerfileName,
		"alice/app",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("repair prompt missing %q:\n%s", want, prompt)
		}
	}
}
