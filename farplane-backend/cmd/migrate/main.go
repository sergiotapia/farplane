package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/db"
)

func main() { //nolint:gocyclo // multi-branch orchestration; keep under threshold when rewriting
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	cmd := os.Args[1]

	switch cmd {
	case "up":
		err = db.MigrateUp(cfg.DatabaseURL)
	case "down":
		err = db.MigrateDown(cfg.DatabaseURL)
	case "reset":
		err = db.MigrateReset(cfg.DatabaseURL)
	case "status":
		err = db.MigrateStatus(cfg.DatabaseURL)
	case "version":
		err = db.MigrateVersion(cfg.DatabaseURL)
	case "create":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: migrate create <name>")
			os.Exit(2)
		}

		dir := migrationsPath()
		err = db.CreateMigration(dir, os.Args[2])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	if err != nil {
		log.Fatalf("migrate %s: %v", cmd, err) //nolint:gosec // G706: cmd is a fixed CLI verb from argv.
	}
}

func migrationsPath() string {
	// Prefer the package migrations dir relative to this module.
	candidates := []string{
		filepath.Join("internal", "db", "migrations"),
		filepath.Join("farplane-backend", "internal", "db", "migrations"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}

	return filepath.Join("internal", "db", "migrations")
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: migrate <command>

Commands:
  up                 Apply all pending migrations
  down               Roll back one migration
  reset              Roll back all migrations
  status             Show migration status
  version            Show current schema version
  create <name>      Create a new SQL migration file

Environment:
  DATABASE_URL       Postgres connection string
                     (default local: farplane_dev on 127.0.0.1:5432)
  APP_ENV            local/dev keep the default DSN; other values require DATABASE_URL
`)
}
