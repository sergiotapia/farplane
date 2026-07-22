package httpapi

import (
	"context"

	"github.com/farplane/farplane/farplane-backend/internal/dockerlint"
)

// runDockerfileLint runs parse + hadolint/docker --check.
// Callers reject the save when ok is false; status is not set by lint.
func runDockerfileLint(ctx context.Context, text string) (ok bool, log string) {
	result := dockerlint.Lint(ctx, text)
	return result.OK, result.Log
}
