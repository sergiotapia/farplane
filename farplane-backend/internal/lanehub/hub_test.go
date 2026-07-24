package lanehub_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/lanehub"
)

func TestStatusClientReceivesTurnUpdates(t *testing.T) {
	t.Parallel()

	hub := lanehub.New()
	client := &lanehub.StatusClient{
		UserID: "user-1",
		Send:   make(chan []byte, 4),
	}

	hub.SubscribeStatus(client)
	defer hub.UnsubscribeStatus(client)

	hub.SetStatusWatches(client, []string{"lane-a", "lane-b"})

	if !hub.TryBeginTurn("lane-a") {
		t.Fatal("expected begin turn")
	}

	select {
	case raw := <-client.Send:
		var msg map[string]any
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if msg["type"] != "turn" || msg["lane_id"] != "lane-a" || msg["turn_running"] != true {
			t.Fatalf("unexpected turn frame: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for turn start")
	}

	hub.EndTurn("lane-a")

	select {
	case raw := <-client.Send:
		var msg map[string]any
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if msg["type"] != "turn" || msg["lane_id"] != "lane-a" || msg["turn_running"] != false {
			t.Fatalf("unexpected idle frame: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for turn end")
	}

	// Unwatched lane should not notify.
	if !hub.TryBeginTurn("lane-c") {
		t.Fatal("expected begin unwatched turn")
	}

	select {
	case raw := <-client.Send:
		t.Fatalf("unexpected frame for unwatched lane: %s", raw)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSubscribeBroadcastAndTurnSnapshot(t *testing.T) {
	t.Parallel()

	hub := lanehub.New()
	client := &lanehub.Client{
		UserID: "user-1",
		Send:   make(chan []byte, 4),
	}

	hub.Subscribe("lane-z", client)

	hub.BroadcastJSON("lane-z", map[string]any{"type": "ping"})

	select {
	case raw := <-client.Send:
		var msg map[string]any
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if msg["type"] != "ping" {
			t.Fatalf("unexpected frame: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast")
	}

	if !hub.TryBeginTurn("lane-z") {
		t.Fatal("expected begin turn")
	}

	snap := hub.TurnSnapshot([]string{"lane-z", "", "lane-other"})
	if !snap["lane-z"] {
		t.Fatalf("expected lane-z busy, got %#v", snap)
	}

	if snap["lane-other"] {
		t.Fatalf("expected lane-other idle, got %#v", snap)
	}

	hub.DropUser("lane-z", "user-1")

	select {
	case _, ok := <-client.Send:
		if ok {
			t.Fatal("expected send channel closed after DropUser")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for DropUser close")
	}
}
