package dockerlint

import "testing"

func TestParseGateRequiresFrom(t *testing.T) {
	t.Parallel()

	if err := parseGate("RUN echo hi"); err == nil {
		t.Fatal("expected lint failure without FROM")
	}
}

func TestParseGateAcceptsValidDockerfile(t *testing.T) {
	t.Parallel()

	if err := parseGate("FROM debian:bookworm-slim\nRUN echo hi\n"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestParseGateRejectsUnknownInstruction(t *testing.T) {
	t.Parallel()

	if err := parseGate("FROM alpine\nFOOBAR baz\n"); err == nil {
		t.Fatal("expected unknown instruction failure")
	}
}

func TestParseGateAllowsBackslashContinuations(t *testing.T) {
	t.Parallel()

	dockerfile := `FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8
RUN apt-get update && apt-get install -y \
        curl \
        git
`
	if err := parseGate(dockerfile); err != nil {
		t.Fatalf("expected continuation lines to be accepted: %v", err)
	}
}
