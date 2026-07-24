package db_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/db"
)

// Serialize tests that share farplane_test.
var testDBMu sync.Mutex

func testDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}

	return "postgres://postgres:postgres@127.0.0.1:5432/farplane_test?sslmode=disable"
}

func requireTestDB(t *testing.T) string {
	t.Helper()
	testDBMu.Lock()
	t.Cleanup(testDBMu.Unlock)

	url := testDatabaseURL()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Skipf("test database unavailable (%s): %v", url, err)
	}

	pool.Close()

	return url
}

func TestOpen(t *testing.T) {
	url := requireTestDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpenEmptyURL(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := db.Open(ctx, ""); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestMigrateUpAndStatus(t *testing.T) {
	url := requireTestDB(t)

	if err := db.MigrateUp(url); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	// Idempotent: second up should succeed with nothing pending.
	if err := db.MigrateUp(url); err != nil {
		t.Fatalf("MigrateUp again: %v", err)
	}

	if err := db.MigrateStatus(url); err != nil {
		t.Fatalf("MigrateStatus: %v", err)
	}

	if err := db.MigrateVersion(url); err != nil {
		t.Fatalf("MigrateVersion: %v", err)
	}
}
