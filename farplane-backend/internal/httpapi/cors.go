package httpapi

import (
	"net/url"
	"slices"
	"strings"

	"github.com/farplane/farplane/farplane-backend/internal/config"
)

// spaOrigins returns browser origins allowed to call the API with credentials.
func spaOrigins(cfg config.Config) []string { //nolint:gocyclo // multi-branch orchestration; keep under threshold when rewriting
	base := strings.TrimRight(strings.TrimSpace(cfg.AppBaseURL), "/")
	if base == "" {
		base = "http://localhost:3000"
	}

	origins := []string{base}

	u, err := url.Parse(base)
	if err != nil {
		return origins
	}

	host := strings.ToLower(u.Hostname())

	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}

	// Local installs often mix localhost and 127.0.0.1.
	if host == "localhost" || host == "127.0.0.1" {
		alts := []string{"localhost", "127.0.0.1"}
		for _, h := range alts {
			origin := u.Scheme + "://" + h
			if (u.Scheme == "http" && port != "80") || (u.Scheme == "https" && port != "443") {
				origin += ":" + port
			}

			if !containsString(origins, origin) {
				origins = append(origins, origin)
			}
		}
	}

	return origins
}

func containsString(items []string, want string) bool {
	return slices.Contains(items, want)
}
