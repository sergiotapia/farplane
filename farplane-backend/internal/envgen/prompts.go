package envgen

import (
	"strconv"
	"strings"
)

// buildInitialPrompt is the first agent turn: explore lightly and write a Dockerfile.
// The agent must not run docker build; Go builds after each turn.
func buildInitialPrompt(repoFullName, outFile, baseFile string) string {
	var b strings.Builder
	b.WriteString("Create a Farplane Project Environment Dockerfile for this repo checkout.\n\n")
	b.WriteString("Farplane is a control plane for AI agent computers. A Project binds one GitHub\n")
	b.WriteString("repo. A Lane is a chat plus one container. Every Lane for this Project boots from\n")
	b.WriteString("ONE Project Environment Dockerfile — a shared dev sandbox (not production).\n\n")
	b.WriteString("Hard limits:\n")
	b.WriteString("- At most 5 tool calls.\n")
	b.WriteString("- Read only manifests when present: mix.exs, config/dev.exs, package.json,\n")
	b.WriteString("  assets/package.json, .tool-versions, go.mod, Gemfile, requirements.txt,\n")
	b.WriteString("  pyproject.toml, Cargo.toml, Dockerfile, docker-compose.yml, and ")
	b.WriteString(baseFile)
	b.WriteString(".\n")
	b.WriteString("- Do not read deps/, _build/, node_modules/, .git/, or large lockfile tails.\n")
	b.WriteString("- Do NOT run docker build. Do NOT ask questions.\n")
	b.WriteString("- Write ONE complete Dockerfile to ")
	b.WriteString(outFile)
	b.WriteString(" and stop.\n\n")
	b.WriteString("Dockerfile rules:\n")
	b.WriteString("- Start from ")
	b.WriteString(baseFile)
	b.WriteString(" in this folder (copy its contents, then add project layers).\n")
	b.WriteString("- Keep COPY bridge, agent CLIs, EXPOSE 7420, and bridge ENTRYPOINT.\n")
	b.WriteString("- Debian bookworm only for OS packages.\n")
	b.WriteString("- Prefer mise for ALL language/runtime installs (Node, Go, Python, Ruby, Rust,\n")
	b.WriteString("  Elixir, Erlang, Java, etc.). Do not use NodeSource, asdf, nvm, pyenv, rbenv,\n")
	b.WriteString("  or apt language packages when mise can install them.\n")
	b.WriteString("- Install mise once, then set:\n")
	b.WriteString("  ENV PATH=\"/root/.local/share/mise/shims:/root/.local/bin:$PATH\"\n")
	b.WriteString("- Pin concrete mise versions (prefer matching .tool-versions / manifests).\n")
	b.WriteString("- For each tool: `mise install <tool>@<version> && mise use --global <tool>@<version>`.\n")
	b.WriteString("- Elixir needs Erlang first: install+use erlang BEFORE elixir (erl must be on PATH).\n")
	b.WriteString("  Example: erlang@27.2 then elixir@1.18.3; apt libncurses-dev libssl-dev first.\n")
	b.WriteString("- Use apt only for OS packages: build-essential, curl, git, lib*-dev, Postgres,\n")
	b.WriteString("  redis-server, inotify-tools, etc. Postgres: apt postgresql + postgresql-client\n")
	b.WriteString("  (not postgresql-16 unless you add PGDG).\n")
	b.WriteString("- Hex: mix archive.install github hexpm/hex branch latest --force\n")
	b.WriteString("  (do not use mix local.hex / mix local.rebar — builds.hex.pm TLS often fails).\n")
	b.WriteString("- Install only what this repo needs.\n")
	if strings.TrimSpace(repoFullName) != "" {
		b.WriteString("\nGitHub repository: ")
		b.WriteString(repoFullName)
		b.WriteString("\n")
	}
	return b.String()
}

// buildRepairPrompt asks the agent to rewrite the Dockerfile using a docker build failure.
func buildRepairPrompt(repoFullName, outFile, buildLog string, attempt, maxAttempts int) string {
	var b strings.Builder
	b.WriteString("Fix the Farplane Project Environment Dockerfile after a failed docker build.\n\n")
	b.WriteString("Hard limits:\n")
	b.WriteString("- At most 5 tool calls.\n")
	b.WriteString("- Read the current ")
	b.WriteString(outFile)
	b.WriteString(" first, then rewrite that same file in place (full file).\n")
	b.WriteString("- Do NOT run docker build. Do NOT ask questions. Stop after writing.\n")
	b.WriteString("- Keep Farplane bridge COPY, agent CLIs, EXPOSE 7420, ENTRYPOINT.\n")
	b.WriteString("- Prefer mise for ALL language/runtime installs. Keep apt for OS packages only\n")
	b.WriteString("  (Postgres, build libs, inotify-tools). Set mise shims on PATH.\n")
	b.WriteString("- If mise/elixir failed: install+use erlang BEFORE elixir, pin patch versions\n")
	b.WriteString("  (erlang@27.2 then elixir@1.18.3), and install libncurses-dev libssl-dev.\n")
	b.WriteString("- Hex via: mix archive.install github hexpm/hex branch latest --force\n")
	b.WriteString("  (avoid mix local.hex / mix local.rebar).\n\n")
	b.WriteString("This is repair attempt ")
	b.WriteString(strconv.Itoa(attempt))
	b.WriteString(" of ")
	b.WriteString(strconv.Itoa(maxAttempts))
	b.WriteString(".\n")
	if strings.TrimSpace(repoFullName) != "" {
		b.WriteString("GitHub repository: ")
		b.WriteString(repoFullName)
		b.WriteString("\n")
	}
	b.WriteString("\nDocker build failure (tail):\n-----\n")
	b.WriteString(strings.TrimSpace(buildLog))
	b.WriteString("\n-----\n")
	return b.String()
}
