package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/db"
)

func TestOpen(t *testing.T) {
	pool := db.OpenIsolated(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	pool := db.OpenIsolated(t)
	url := pool.Config().ConnString()

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

func TestOpenIsolatedAreIndependent(t *testing.T) {
	a := db.OpenIsolated(t)
	b := db.OpenIsolated(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := a.Exec(ctx, `
		INSERT INTO organizations (name) VALUES ('only-a')
	`); err != nil {
		t.Fatalf("insert into A: %v", err)
	}

	var exists bool
	if err := b.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organizations WHERE name = 'only-a'
		)
	`).Scan(&exists); err != nil {
		t.Fatalf("query B: %v", err)
	}

	if exists {
		t.Fatal("isolated database B saw rows from A")
	}
}
