package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Cannot use t.Parallel: this test mutates process environment via t.Setenv.
	tests := []struct {
		name     string
		env      map[string]string
		wantAddr string
		wantGin  string
	}{
		{
			name:     "defaults",
			wantAddr: ":8080",
		},
		{
			name:     "ADDR wins over PORT",
			env:      map[string]string{"ADDR": "127.0.0.1:9090", "PORT": "3000"},
			wantAddr: "127.0.0.1:9090",
		},
		{
			name:     "PORT without colon",
			env:      map[string]string{"PORT": "9090"},
			wantAddr: ":9090",
		},
		{
			name:     "PORT with leading colon",
			env:      map[string]string{"PORT": ":9090"},
			wantAddr: ":9090",
		},
		{
			name:     "GIN_MODE release",
			env:      map[string]string{"GIN_MODE": "release"},
			wantAddr: ":8080",
			wantGin:  "release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ADDR", "")
			t.Setenv("PORT", "")
			t.Setenv("GIN_MODE", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg := Load()

			if cfg.Addr != tt.wantAddr {
				t.Fatalf("Addr = %q, want %q", cfg.Addr, tt.wantAddr)
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
		})
	}
}
