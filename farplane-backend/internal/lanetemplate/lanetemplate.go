// Package lanetemplate embeds the default Farplane Lane Dockerfile and bridge.
package lanetemplate

import (
	"embed"
	"io/fs"
)

//go:embed Dockerfile bridge/*
var files embed.FS

// DefaultDockerfile is the seeded Farplane default Lane Dockerfile text.
func DefaultDockerfile() (string, error) {
	b, err := files.ReadFile("Dockerfile")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildContextFS returns the files that must sit beside the Dockerfile for docker build
// (bridge/ and any other COPY sources).
func BuildContextFS() fs.FS {
	return files
}
