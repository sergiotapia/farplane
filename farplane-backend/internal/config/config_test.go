package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Cannot use t.Parallel: this test mutates process environment via t.Setenv.
	defaultDB := "postgres://postgres:postgres@127.0.0.1:5432/farplane_dev?sslmode=disable"

	tests := []struct {
		name     string
		env      map[string]string
		wantAddr string
		wantDB   string
		wantEnv  string
		wantGin  string
		wantErr  bool
	}{
		{
			name:     "defaults",
			wantAddr: ":8080",
			wantDB:   defaultDB,
		},
		{
			name:     "ADDR wins over PORT",
			env:      map[string]string{"ADDR": "127.0.0.1:9090", "PORT": "3000"},
			wantAddr: "127.0.0.1:9090",
			wantDB:   defaultDB,
		},
		{
			name:     "PORT without colon",
			env:      map[string]string{"PORT": "9090"},
			wantAddr: ":9090",
			wantDB:   defaultDB,
		},
		{
			name:     "PORT with leading colon",
			env:      map[string]string{"PORT": ":9090"},
			wantAddr: ":9090",
			wantDB:   defaultDB,
		},
		{
			name:     "GIN_MODE release",
			env:      map[string]string{"GIN_MODE": "release"},
			wantAddr: ":8080",
			wantDB:   defaultDB,
			wantGin:  "release",
		},
		{
			name: "DATABASE_URL override",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@db:5432/farplane?sslmode=require",
			},
			wantAddr: ":8080",
			wantDB:   "postgres://user:pass@db:5432/farplane?sslmode=require",
		},
		{
			name: "local APP_ENV keeps default DSN",
			env: map[string]string{
				"APP_ENV": "local",
			},
			wantAddr: ":8080",
			wantDB:   defaultDB,
			wantEnv:  "local",
		},
		{
			name: "production requires DATABASE_URL",
			env: map[string]string{
				"APP_ENV": "production",
			},
			wantErr: true,
		},
		{
			name: "production requires SESSION_SECRET",
			env: map[string]string{
				"APP_ENV":      "production",
				"DATABASE_URL": "postgres://user:pass@db:5432/farplane?sslmode=require",
			},
			wantErr: true,
		},
		{
			name: "production with DATABASE_URL and SESSION_SECRET",
			env: map[string]string{
				"APP_ENV":        "production",
				"DATABASE_URL":   "postgres://user:pass@db:5432/farplane?sslmode=require",
				"SESSION_SECRET": "prod-session-secret",
			},
			wantAddr: ":8080",
			wantDB:   "postgres://user:pass@db:5432/farplane?sslmode=require",
			wantEnv:  "production",
		},
		{
			name: "google oauth and session cookie env",
			env: map[string]string{
				"GOOGLE_CLIENT_ID":      "client-id",
				"GOOGLE_CLIENT_SECRET":  "client-secret",
				"GOOGLE_REDIRECT_URL":   "http://localhost:8080/api/v1/auth/google/callback",
				"APP_BASE_URL":          "http://localhost:3000/",
				"SESSION_COOKIE_SECURE": "false",
				"SESSION_SECRET":        "custom-secret",
			},
			wantAddr: ":8080",
			wantDB:   defaultDB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ADDR", "")
			t.Setenv("PORT", "")
			t.Setenv("GIN_MODE", "")
			t.Setenv("DATABASE_URL", "")
			t.Setenv("APP_ENV", "")
			t.Setenv("SESSION_SECRET", "")
			t.Setenv("SESSION_COOKIE_SECURE", "")
			t.Setenv("GOOGLE_CLIENT_ID", "")
			t.Setenv("GOOGLE_CLIENT_SECRET", "")
			t.Setenv("GOOGLE_REDIRECT_URL", "")
			t.Setenv("APP_BASE_URL", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if cfg.Addr != tt.wantAddr {
				t.Fatalf("Addr = %q, want %q", cfg.Addr, tt.wantAddr)
			}
			if cfg.DatabaseURL != tt.wantDB {
				t.Fatalf("DatabaseURL = %q, want %q", cfg.DatabaseURL, tt.wantDB)
			}
			if cfg.AppEnv != tt.wantEnv {
				t.Fatalf("AppEnv = %q, want %q", cfg.AppEnv, tt.wantEnv)
			}
			if cfg.GinMode != tt.wantGin {
				t.Fatalf("GinMode = %q, want %q", cfg.GinMode, tt.wantGin)
			}
			if cfg.ReadHeaderTimeout != 5*time.Second {
				t.Fatalf("ReadHeaderTimeout = %v, want 5s", cfg.ReadHeaderTimeout)
			}
			if cfg.ReadTimeout != 15*time.Second {
				t.Fatalf("ReadTimeout = %v, want 15s", cfg.ReadTimeout)
			}
			if cfg.WriteTimeout != 30*time.Second {
				t.Fatalf("WriteTimeout = %v, want 30s", cfg.WriteTimeout)
			}
			if cfg.IdleTimeout != 60*time.Second {
				t.Fatalf("IdleTimeout = %v, want 60s", cfg.IdleTimeout)
			}
			if cfg.ShutdownTimeout != 10*time.Second {
				t.Fatalf("ShutdownTimeout = %v, want 10s", cfg.ShutdownTimeout)
			}
			if cfg.SessionSecret == "" {
				t.Fatal("SessionSecret is empty")
			}
			if tt.name == "google oauth and session cookie env" {
				if !cfg.GoogleOAuthConfigured() {
					t.Fatal("expected GoogleOAuthConfigured true")
				}
				if cfg.AppBaseURL != "http://localhost:3000" {
					t.Fatalf("AppBaseURL = %q, want trimmed base", cfg.AppBaseURL)
				}
				if cfg.SessionCookieSecure {
					t.Fatal("SessionCookieSecure = true, want false")
				}
				if cfg.SessionSecret != "custom-secret" {
					t.Fatalf("SessionSecret = %q", cfg.SessionSecret)
				}
			}
			if tt.name == "production with DATABASE_URL and SESSION_SECRET" {
				if !cfg.SessionCookieSecure {
					t.Fatal("SessionCookieSecure = false, want true for production default")
				}
			}
		})
	}
}
