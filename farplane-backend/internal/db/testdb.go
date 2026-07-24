package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	//nolint:gosec // G101: local-only default DSN; override with TEST_DATABASE_URL.
	defaultTestDatabaseURL = "postgres://postgres:postgres@127.0.0.1:5432/farplane_test?sslmode=disable"
	templateDatabaseName   = "farplane_test_template"
	// Arbitrary key so parallel go test processes serialize template setup.
	templateAdvisoryLockKey int64 = 0x666172706c616e65 // "farplane"
)

var (
	ensureTemplateOnce sync.Once
	errEnsureTemplate  error
)

// TestDatabaseURL returns TEST_DATABASE_URL or the local farplane_test default.
func TestDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}

	return defaultTestDatabaseURL
}

// OpenIsolated returns a pool to a fresh database cloned from a migrated
// template. Migrations run once per process (and are locked across processes).
// The clone is dropped when the test ends.
//
// Nested transactions and concurrent commits work as in production — this is
// not an outer-transaction sandbox (those break pgx.BeginFunc and races that
// must actually commit).
func OpenIsolated(tb testing.TB) *pgxpool.Pool {
	tb.Helper()

	pool, cleanup := prepareIsolated(tb)
	tb.Cleanup(cleanup)

	return pool
}

// PrepareIsolated is like OpenIsolated but also returns cleanup for suites that
// open more than one database inside a single test (for example godog).
// Calling cleanup more than once is safe. The test still drops the DB on end.
func PrepareIsolated(tb testing.TB) (*pgxpool.Pool, func()) {
	tb.Helper()

	pool, cleanup := prepareIsolated(tb)
	tb.Cleanup(cleanup)

	return pool, cleanup
}

func prepareIsolated(tb testing.TB) (*pgxpool.Pool, func()) {
	tb.Helper()

	baseURL := TestDatabaseURL()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()

	if err := pingAdmin(pingCtx, baseURL); err != nil {
		tb.Skipf("test database unavailable: %v", err)
	}

	if err := ensureTemplate(baseURL); err != nil {
		tb.Fatalf("test template database: %v", err)
	}

	name := fmt.Sprintf(
		"farplane_test_%d_%d",
		os.Getpid(),
		time.Now().UnixNano(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	tb.Cleanup(cancel)

	if err := createDatabaseFromTemplate(ctx, baseURL, name); err != nil {
		tb.Fatalf("create isolated test database: %v", err)
	}

	dbURL, err := withDatabaseName(baseURL, name)
	if err != nil {
		tb.Fatalf("isolated database URL: %v", err)
	}

	pool, err := Open(ctx, dbURL)
	if err != nil {
		_ = dropDatabase(context.Background(), baseURL, name)

		tb.Fatalf("open isolated test database: %v", err)
	}

	var once sync.Once

	cleanup := func() {
		once.Do(func() {
			pool.Close()

			dropCtx, dropCancel := context.WithTimeout(
				context.Background(),
				15*time.Second,
			)
			defer dropCancel()

			if err := dropDatabase(dropCtx, baseURL, name); err != nil {
				tb.Errorf("drop isolated test database %s: %v", name, err)
			}
		})
	}

	return pool, cleanup
}

func ensureTemplate(baseURL string) error {
	ensureTemplateOnce.Do(func() {
		errEnsureTemplate = ensureTemplateLocked(baseURL)
	})

	return errEnsureTemplate
}

func ensureTemplateLocked(baseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	adminURL, err := withDatabaseName(baseURL, "postgres")
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect admin database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, templateAdvisoryLockKey); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, templateAdvisoryLockKey)
	}()

	if err := ensureTemplateDatabase(ctx, conn, baseURL); err != nil {
		return err
	}

	if err := terminateDatabaseBackends(ctx, conn, templateDatabaseName); err != nil {
		return err
	}

	if _, err := conn.Exec(ctx, `
		UPDATE pg_database
		SET datistemplate = true
		WHERE datname = $1
	`, templateDatabaseName); err != nil {
		return fmt.Errorf("mark template database: %w", err)
	}

	return nil
}

func ensureTemplateDatabase(
	ctx context.Context,
	conn *pgx.Conn,
	baseURL string,
) error {
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_database WHERE datname = $1
		)
	`, templateDatabaseName).Scan(&exists); err != nil {
		return fmt.Errorf("check template database: %w", err)
	}

	if !exists {
		createSQL := "CREATE DATABASE " + pgx.Identifier{templateDatabaseName}.Sanitize()
		if _, err := conn.Exec(ctx, createSQL); err != nil {
			return fmt.Errorf("create template database: %w", err)
		}
	}

	templateURL, err := withDatabaseName(baseURL, templateDatabaseName)
	if err != nil {
		return err
	}

	// Allow connections while migrating; clear template flag if a prior run set it.
	if _, err := conn.Exec(ctx, `
		UPDATE pg_database
		SET datistemplate = false
		WHERE datname = $1
	`, templateDatabaseName); err != nil {
		return fmt.Errorf("clear template flag: %w", err)
	}

	if err := MigrateUp(templateURL); err != nil {
		return fmt.Errorf("migrate template database: %w", err)
	}

	return nil
}

func createDatabaseFromTemplate(ctx context.Context, baseURL, name string) error {
	adminURL, err := withDatabaseName(baseURL, "postgres")
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect admin database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := terminateDatabaseBackends(ctx, conn, templateDatabaseName); err != nil {
		return err
	}

	createSQL := "CREATE DATABASE " +
		pgx.Identifier{name}.Sanitize() +
		" TEMPLATE " +
		pgx.Identifier{templateDatabaseName}.Sanitize()
	if _, err := conn.Exec(ctx, createSQL); err != nil {
		return fmt.Errorf("create database from template: %w", err)
	}

	return nil
}

func dropDatabase(ctx context.Context, baseURL, name string) error {
	adminURL, err := withDatabaseName(baseURL, "postgres")
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect admin database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	dropSQL := "DROP DATABASE IF EXISTS " +
		pgx.Identifier{name}.Sanitize() +
		" WITH (FORCE)"
	if _, err := conn.Exec(ctx, dropSQL); err != nil {
		return fmt.Errorf("drop database: %w", err)
	}

	return nil
}

func terminateDatabaseBackends(
	ctx context.Context,
	conn *pgx.Conn,
	databaseName string,
) error {
	if _, err := conn.Exec(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1
		  AND pid <> pg_backend_pid()
	`, databaseName); err != nil {
		return fmt.Errorf("terminate backends on %s: %w", databaseName, err)
	}

	return nil
}

func pingAdmin(ctx context.Context, baseURL string) error {
	adminURL, err := withDatabaseName(baseURL, "postgres")
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return err
	}

	defer func() { _ = conn.Close(ctx) }()

	return conn.Ping(ctx)
}

func withDatabaseName(databaseURL, name string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}

	if u.Scheme == "" {
		return "", errors.New("parse database URL: missing scheme")
	}

	u.Path = "/" + strings.TrimPrefix(name, "/")
	u.RawPath = ""

	return u.String(), nil
}
