package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

const (
	migrationsDir         = "migrations"
	defaultMigrateTimeout = 30 * time.Second
)

// openSQL opens a database/sql handle for goose (pgx stdlib driver).
func openSQL(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, errors.New("database URL is empty")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open sql db: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sql db: %w", err)
	}

	return db, nil
}

func withProvider(databaseURL string, fn func(context.Context, *goose.Provider) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultMigrateTimeout)
	defer cancel()

	db, err := openSQL(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	fsys, err := fs.Sub(embedMigrations, migrationsDir)
	if err != nil {
		return fmt.Errorf("migrations fs: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	defer func() { _ = provider.Close() }()

	return fn(ctx, provider)
}

// MigrateUp applies all pending embedded migrations.
func MigrateUp(databaseURL string) error {
	return withProvider(databaseURL, func(ctx context.Context, p *goose.Provider) error {
		if _, err := p.Up(ctx); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}

		return nil
	})
}

// MigrateDown rolls back one migration.
func MigrateDown(databaseURL string) error {
	return withProvider(databaseURL, func(ctx context.Context, p *goose.Provider) error {
		if _, err := p.Down(ctx); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}

		return nil
	})
}

// MigrateReset clears the public schema so the next MigrateUp applies a clean
// history. Local databases may be dropped and recreated at any time; rolling
// back each Down migration is brittle when a prior run left a half-applied
// schema (common under concurrent package tests).
func MigrateReset(databaseURL string) error {
	if err := recreatePublicSchema(databaseURL); err != nil {
		return fmt.Errorf("migrate reset: %w", err)
	}

	return nil
}

func recreatePublicSchema(databaseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultMigrateTimeout)
	defer cancel()

	sqlDB, err := openSQL(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	_, err = sqlDB.ExecContext(ctx, `
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
		GRANT ALL ON SCHEMA public TO CURRENT_USER;
		GRANT ALL ON SCHEMA public TO public;
	`)
	if err != nil {
		return fmt.Errorf("recreate public schema: %w", err)
	}

	return nil
}

// MigrateStatus prints migration status.
func MigrateStatus(databaseURL string) error {
	return withProvider(databaseURL, func(ctx context.Context, p *goose.Provider) error {
		statuses, err := p.Status(ctx)
		if err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}

		fmt.Println("    Applied At                  Migration")
		fmt.Println("    =======================================")

		for _, s := range statuses {
			applied := "Pending"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.Format(time.ANSIC)
			}

			name := ""
			if s.Source != nil {
				name = s.Source.Path
			}

			fmt.Printf("    %-28s -- %s\n", applied, name)
		}

		return nil
	})
}

// MigrateVersion prints the current schema version.
func MigrateVersion(databaseURL string) error {
	return withProvider(databaseURL, func(ctx context.Context, p *goose.Provider) error {
		version, err := p.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("migrate version: %w", err)
		}

		fmt.Printf("goose: version %d\n", version)

		return nil
	})
}

// CreateMigration writes a new empty SQL migration on disk (not embedded until rebuild).
// dir should be the on-disk migrations path (for example farplane-backend/internal/db/migrations).
func CreateMigration(dir, name string) error {
	if name == "" {
		return errors.New("migration name is required")
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create migrations dir: %w", err)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve migrations dir: %w", err)
	}

	if err := goose.Create(nil, abs, name, "sql"); err != nil {
		return fmt.Errorf("create migration: %w", err)
	}

	return nil
}
