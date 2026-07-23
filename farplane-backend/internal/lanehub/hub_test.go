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
