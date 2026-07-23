package models

import (
	"encoding/json"
	"time"
)

// Lane template validation statuses (set only by Validate build).
const (
	LaneTemplateValidationValid   = "valid"
	LaneTemplateValidationInvalid = "invalid"
)

// Runtime kinds for Lane computers.
const (
	RuntimeKindDocker  = "docker"
	RuntimeKindSprites = "sprites"
)

// Agent providers that can drive a Lane.
const (
	AgentProviderClaudeCode = "claude_code"
	AgentProviderCodex      = "codex"
	AgentProviderOpenCode   = "opencode"
	AgentProviderOhMyPi     = "oh_my_pi"
)

// Lane lifecycle statuses.
const (
	LaneStatusCreating  = "creating"
	LaneStatusRunning   = "running"
	LaneStatusSleeping  = "sleeping"
	LaneStatusError     = "error"
	LaneStatusDestroyed = "destroyed"
)

// Lane kinds: project-backed or scratch (no Project).
const (
	LaneKindProject = "project"
	LaneKindScratch = "scratch"
)

// Lane participant roles.
const (
	LaneParticipantRoleOwner       = "owner"
	LaneParticipantRoleParticipant = "participant"
)

// Well-known organization secret names.
const (
	SecretNameAnthropicAPIKey  = "ANTHROPIC_API_KEY"
	SecretNameOpenAIAPIKey     = "OPENAI_API_KEY"
	SecretNameOpenRouterAPIKey = "OPENROUTER_API_KEY"
)

// Lane message event types.
const (
	LaneEventUserMessage        = "user_message"
	LaneEventAssistantMessage   = "assistant_message"
	LaneEventReasoning          = "reasoning"
	LaneEventToolCall           = "tool_call"
	LaneEventToolResult         = "tool_result"
	LaneEventToolProgress       = "tool_progress"
	LaneEventFileDiff           = "file_diff"
	LaneEventPermissionRequest  = "permission_request"
	LaneEventPermissionResponse = "permission_response"
	LaneEventStatus             = "status"
	LaneEventProviderSession    = "provider_session"
	LaneEventParticipantJoined  = "participant_joined"
	LaneEventParticipantRemoved = "participant_removed"
	LaneEventAgentChanged       = "agent_changed"
)

// Message roles on the Lane timeline.
const (
	LaneMessageRoleUser      = "user"
	LaneMessageRoleAssistant = "assistant"
	LaneMessageRoleSystem    = "system"
)

// LaneTemplate is an org-editable Dockerfile used to spawn Lanes.
type LaneTemplate struct {
	ID                      string
	OrganizationID          string
	Name                    string
	Description             string
	DockerfileText          string
	IsSystemDefault         bool
	ForkedFromTemplateID    *string
	CreatedByUserID         *string
	UpdatedByUserID         *string
	ValidationStatus        string
	ValidatedImageReference *string
	LastValidationLog       *string
	ValidatedAt             *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// OrganizationSecretMeta is a secret name without the decrypted value.
type OrganizationSecretMeta struct {
	Name      string
	IsSet     bool
	UpdatedAt *time.Time
}

// Lane is one shared chat thread plus one Runtime computer.
type Lane struct {
	ID                     string
	ProjectID              *string
	OrganizationID         string
	OwnerUserID            string
	Name                   string
	LaneKind               string
	LaneTemplateID         *string
	DockerfileSnapshot     string
	ImageReference         *string
	RuntimeKind            string
	RuntimeID              *string
	AgentProvider          string
	AgentProviderSessionID *string
	ModelSource            string
	AgentModel             string
	ReasoningEffort        *string
	Status                 string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	// HasOtherParticipants is set by list queries when another seat exists.
	HasOtherParticipants bool
}

// LaneParticipant is a seat on a Lane chat.
type LaneParticipant struct {
	ID       string
	LaneID   string
	UserID   string
	Role     string
	JoinedAt time.Time
}

// LaneInvite is an open multi-use share link for a Lane.
type LaneInvite struct {
	ID              string
	LaneID          string
	Token           string
	InvitedByUserID *string
	ExpiresAt       *time.Time
	RevokedAt       *time.Time
	CreatedAt       time.Time
}

// IsPending reports whether the invite can still be accepted.
func (i LaneInvite) IsPending(now time.Time) bool {
	if i.RevokedAt != nil {
		return false
	}
	if i.ExpiresAt != nil && !i.ExpiresAt.After(now) {
		return false
	}
	return true
}

// LaneMessage is one timeline event on a Lane.
type LaneMessage struct {
	ID             string
	LaneID         string
	SequenceNumber int64
	EventType      string
	Role           *string
	AuthorUserID   *string
	Body           *string
	Payload        json.RawMessage
	CreatedAt      time.Time
}
