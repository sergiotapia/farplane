package lanetemplate_test

import (
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/lanetemplate"
)

func TestDefaultDockerfile(t *testing.T) {
	t.Parallel()

	text, err := lanetemplate.DefaultDockerfile()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(text, "FROM ") {
		t.Fatal("dockerfile missing FROM")
	}

	if !strings.Contains(text, "bridge") {
		t.Fatal("dockerfile should reference bridge")
	}
}
