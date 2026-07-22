// Package runtime defines the Lane computer backend interface (Docker now, Sprites later).
package runtime

import (
	"context"
	"io"
)

// CreateRequest creates a new Lane computer instance from an image.
type CreateRequest struct {
	LaneID         string
	ImageReference string
	Name           string
	Env            map[string]string
	Labels         map[string]string
}

// ExecRequest runs a command inside an instance.
type ExecRequest struct {
	Command []string
	Env     map[string]string
	WorkDir string
}

// Instance is a created Runtime computer.
type Instance struct {
	ID        string
	Status    string
	BridgeURL string // base URL for the agent bridge when known
}

// ExecSession is a running exec (stdout/stderr stream).
type ExecSession interface {
	Wait() (exitCode int, err error)
	Stdout() io.Reader
	Stderr() io.Reader
	Close() error
}

// AgentStream is a live stream of normalized bridge events.
type AgentStream interface {
	Events() <-chan AgentEvent
	Close() error
}

// AgentEvent is one normalized event from the in-Lane bridge.
type AgentEvent struct {
	Type              string         `json:"type"`
	Role              string         `json:"role,omitempty"`
	Body              string         `json:"body,omitempty"`
	Status            string         `json:"status,omitempty"`
	ProviderSessionID string         `json:"provider_session_id,omitempty"`
	Done              bool           `json:"done,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
}

// UserTurn is forwarded to the bridge (not the full Postgres history).
type UserTurn struct {
	Type              string `json:"type"`
	LaneID            string `json:"lane_id"`
	MessageID         string `json:"message_id"`
	AuthorUserID      string `json:"author_user_id"`
	AuthorDisplayName string `json:"author_display_name"`
	Text              string `json:"text"`
	Provider          string `json:"provider"`
	ProviderSessionID string `json:"provider_session_id,omitempty"`
	Transcript        string `json:"transcript,omitempty"` // handoff only
}

// Runtime hosts Lane computers behind a single interface.
type Runtime interface {
	Create(ctx context.Context, req CreateRequest) (Instance, error)
	Destroy(ctx context.Context, id string) error
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Exec(ctx context.Context, id string, cmd ExecRequest) (ExecSession, error)
	InjectSecrets(ctx context.Context, id string, secrets map[string]string) error
	PreviewURL(ctx context.Context, id string, port int) (string, error)
	EnsureAgentBridge(ctx context.Context, id string) error
	OpenAgentStream(ctx context.Context, id string) (AgentStream, error)
	SendUserTurn(ctx context.Context, id string, turn UserTurn) error
	InterruptTurn(ctx context.Context, id string) error
	BuildImage(ctx context.Context, dockerfileText string, tag string) (imageReference string, logText string, err error)
}
