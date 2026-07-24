package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// InsertLaneMessageInput appends one timeline event.
type InsertLaneMessageInput struct {
	LaneID       string
	EventType    string
	Role         *string
	AuthorUserID *string
	Body         *string
	Payload      json.RawMessage
}

// InsertLaneMessage appends a message with the next sequence number.
func (s *Store) InsertLaneMessage(ctx context.Context, in InsertLaneMessageInput) (models.LaneMessage, error) {
	now := time.Now().UTC()

	payload := in.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	const q = `
		INSERT INTO lane_messages (
			lane_id, sequence_number, event_type, role, author_user_id, body, payload, created_at
		)
		SELECT $1,
			COALESCE((SELECT MAX(sequence_number) FROM lane_messages WHERE lane_id = $1), 0) + 1,
			$2, $3, $4, $5, $6::jsonb, $7
		RETURNING id, lane_id, sequence_number, event_type, role, author_user_id, body, payload, created_at
	`

	msg, err := scanLaneMessage(s.pool.QueryRow(
		ctx, q, in.LaneID, in.EventType, in.Role, in.AuthorUserID, in.Body, string(payload), now,
	))
	if err != nil {
		return models.LaneMessage{}, fmt.Errorf("insert lane message: %w", err)
	}

	return msg, nil
}

// ListLaneMessages returns timeline events ascending by sequence.
func (s *Store) ListLaneMessages(ctx context.Context, laneID string, afterSequence int64, limit int) ([]models.LaneMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	const q = `
		SELECT id, lane_id, sequence_number, event_type, role, author_user_id, body, payload, created_at
		FROM lane_messages
		WHERE lane_id = $1 AND sequence_number > $2
		ORDER BY sequence_number ASC
		LIMIT $3
	`

	rows, err := s.pool.Query(ctx, q, laneID, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("list lane messages: %w", err)
	}
	defer rows.Close()

	var out []models.LaneMessage

	for rows.Next() {
		msg, err := scanLaneMessage(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, msg)
	}

	return out, rows.Err()
}

// BuildHandoffTranscript builds a compact chat handoff for agent switch.
func (s *Store) BuildHandoffTranscript(ctx context.Context, laneID string, maxTurns int) (string, error) { //nolint:gocyclo,funlen // multi-branch orchestration; keep under threshold when rewriting
	if maxTurns <= 0 {
		maxTurns = 40
	}
	// Fetch more rows than maxTurns so tool notes can sit beside chat turns.
	limit := max(maxTurns*3, 60)

	const q = `
		SELECT event_type, role, body, payload, created_at
		FROM lane_messages
		WHERE lane_id = $1
			AND event_type IN (
				'user_message', 'assistant_message', 'agent_changed',
				'tool_call', 'tool_result'
			)
		ORDER BY sequence_number DESC
		LIMIT $2
	`

	rows, err := s.pool.Query(ctx, q, laneID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	type line struct {
		eventType string
		role      *string
		body      *string
		payload   []byte
	}

	var rev []line

	for rows.Next() {
		var (
			eventType string
			role      *string
			body      *string
			payload   []byte
			createdAt time.Time
		)
		if err := rows.Scan(&eventType, &role, &body, &payload, &createdAt); err != nil {
			return "", err
		}

		rev = append(rev, line{eventType: eventType, role: role, body: body, payload: payload})
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("You are taking over this Farplane Lane. Workspace is already checked out.\n")
	b.WriteString("Prior agents and humans said the following:\n\n")

	chatTurns := 0

	for _, v := range slices.Backward(rev) {
		l := v

		text := ""
		if l.body != nil {
			text = strings.TrimSpace(*l.body)
		}

		switch l.eventType {
		case models.LaneEventUserMessage:
			if chatTurns >= maxTurns {
				continue
			}

			chatTurns++

			if text == "" {
				continue
			}

			b.WriteString("User: ")
			b.WriteString(text)
			b.WriteString("\n\n")
		case models.LaneEventAssistantMessage:
			if chatTurns >= maxTurns {
				continue
			}

			chatTurns++

			if text == "" {
				continue
			}

			b.WriteString("Assistant: ")
			b.WriteString(text)
			b.WriteString("\n\n")
		case models.LaneEventAgentChanged:
			if text == "" {
				continue
			}

			b.WriteString("System: ")
			b.WriteString(text)
			b.WriteString("\n\n")
		case models.LaneEventToolCall, models.LaneEventToolResult:
			note := compactToolNote(l.eventType, text, l.payload)
			if note == "" {
				continue
			}

			b.WriteString("Tool: ")
			b.WriteString(note)
			b.WriteString("\n\n")
		}
	}

	return b.String(), nil
}

func compactToolNote(eventType, body string, payload []byte) string {
	if body != "" {
		runes := []rune(body)
		if len(runes) > 240 {
			return string(runes[:240]) + "…"
		}

		return body
	}

	if len(payload) == 0 {
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return eventType
	}

	if name, ok := m["tool"].(string); ok && name != "" {
		return eventType + " " + name
	}

	if name, ok := m["name"].(string); ok && name != "" {
		return eventType + " " + name
	}

	return eventType
}

func scanLaneMessage(row scannable) (models.LaneMessage, error) {
	var (
		m       models.LaneMessage
		payload []byte
	)

	err := row.Scan(
		&m.ID, &m.LaneID, &m.SequenceNumber, &m.EventType, &m.Role, &m.AuthorUserID, &m.Body, &payload, &m.CreatedAt,
	)
	if err != nil {
		return models.LaneMessage{}, err
	}

	if len(payload) == 0 {
		m.Payload = json.RawMessage(`{}`)
	} else {
		m.Payload = json.RawMessage(payload)
	}

	m.CreatedAt = m.CreatedAt.UTC()

	return m, nil
}
