package dockerlint

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestParseGateRapidValidDockerfiles(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		image := rapid.StringMatching(`[a-z0-9][a-z0-9._/-]{0,40}`).Draw(t, "image")
		extra := rapid.SliceOf(
			rapid.SampledFrom([]string{
				"RUN echo hi",
				"WORKDIR /app",
				"ENV FOO=bar",
				"COPY . /src",
				"USER root",
			}),
		).Draw(t, "extra")

		var b strings.Builder
		b.WriteString("FROM ")
		b.WriteString(image)
		b.WriteByte('\n')

		for _, line := range extra {
			b.WriteString(line)
			b.WriteByte('\n')
		}

		if err := parseGate(b.String()); err != nil {
			t.Fatalf("parseGate rejected valid dockerfile: %v\n%s", err, b.String())
		}
	})
}

func TestParseGateRapidRejectsUnknownInstructions(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		bad := rapid.StringMatching(`[A-Z]{3,12}`).Filter(func(s string) bool {
			_, ok := knownInstructions[s]
			return !ok
		}).Draw(t, "bad")

		text := "FROM alpine\n" + bad + " oops\n"
		if err := parseGate(text); err == nil {
			t.Fatalf("expected unknown instruction failure for %q", bad)
		}
	})
}

func TestParseGateEmptyAndNoFrom(t *testing.T) {
	t.Parallel()

	if err := parseGate(""); err == nil {
		t.Fatal("expected empty failure")
	}

	if err := parseGate("# only comments\n"); err == nil {
		t.Fatal("expected missing FROM failure")
	}
}

func FuzzParseGate(f *testing.F) {
	f.Add("FROM alpine\nRUN echo hi\n")
	f.Add("RUN echo hi\n")
	f.Add("FROM\n")
	f.Add("FROM alpine\nFOOBAR x\n")
	f.Add("# comment\nFROM debian:bookworm-slim\nENV A=1 \\\n    B=2\n")
	f.Fuzz(func(t *testing.T, text string) {
		_ = parseGate(text) // must not panic
	})
}
