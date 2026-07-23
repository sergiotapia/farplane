package envgen_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/envgen"
)

func TestGenerateHeuristicFromGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := envgen.New()
	svc.HTTP = nil // force heuristic path
	result, err := svc.Generate(context.Background(), envgen.Request{
		WorkspaceDir: dir,
		RepoFullName: "alice/app",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(result.DockerfileText, "FROM debian:bookworm-slim") {
		t.Fatalf("dockerfile missing Farplane base: %s", result.DockerfileText)
	}
	if !strings.Contains(result.DockerfileText, "golang-go") {
		t.Fatalf("dockerfile missing go toolchain: %s", result.DockerfileText)
	}
	if !strings.Contains(result.DockerfileText, "/opt/farplane/bridge") {
		t.Fatalf("dockerfile missing bridge: %s", result.DockerfileText)
	}
	if !strings.Contains(result.Log, "found go.mod") {
		t.Fatalf("log = %q", result.Log)
	}
}
