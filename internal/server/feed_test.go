package server

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
)

// TestFeed checks rules FD2, FD3, FD5, and FD6 of the feed spec: one entry
// per message, ended turn, connection, error, exit, and module line, each
// published, kept, and journaled, and the run's cost in the snapshot.
func TestFeed(t *testing.T) {
	s, _ := newTestServer(t)
	var journal bytes.Buffer
	s.FeedTo(&journal)
	events, cancel := s.Subscribe()
	defer cancel()
	<-events // the snapshot

	s.Send("user", post("@lead plan the coupons"))
	s.Send("writer", dm("critic", "a private word"))
	s.SetStatus("writer", api.Status{State: api.StateIdle, Cost: 0.5})
	s.SetStatus("writer", api.Status{State: api.StateWorking})
	s.SetStatus("writer", api.Status{State: api.StateIdle, Cost: 0.25})
	s.AddActivity("writer", api.Activity{ID: 1, Turn: 1, Kind: api.ActivityTurn, Text: "woken by user in the room"})
	s.AddActivity("writer", api.Activity{ID: 2, Turn: 1, Kind: api.ActivityTool, Tool: "Edit", Input: json.RawMessage(`{"file_path":"a/b.go"}`), Done: true})
	s.AddActivity("writer", api.Activity{ID: 3, Turn: 1, Kind: api.ActivityTool, Tool: "Edit", Input: json.RawMessage(`{"file_path":"a/b.go"}`), Done: true})
	s.AddActivity("writer", api.Activity{ID: 4, Turn: 1, Kind: api.ActivityDone, Text: "Done.", Done: true, Duration: 2500, Cost: 0.03})
	s.AddActivity("critic", api.Activity{ID: 5, Turn: 1, Kind: api.ActivityDone, Done: true, Error: true})
	s.SetStatus("critic", api.Status{State: api.StateError, Detail: "claude: boom"})
	s.SetStatus("critic", api.Status{State: api.StateError, Detail: "claude: boom"})
	s.SetStatus("lead", api.Status{State: api.StateExited})
	s.SetStatus("lead", api.Status{State: api.StateExited, Detail: "killed"})
	(&moduleHost{s: s, name: "tasks"}).Feed("[[cancel]] orders set the part of orders to done (was testing)", "cancel")
	s.Step("Connected 3 agents", nil)

	want := []api.Entry{
		{Kind: api.EntryMessage, Who: "user", Text: "@lead plan the coupons", Message: 1},
		{Kind: api.EntryMessage, Who: "writer", To: "critic", Text: "a private word", Message: 2},
		{Kind: api.EntryStatus, Who: "writer", Text: "connected"},
		{Kind: api.EntryTurn, Who: "writer", Text: "turn done · 2.5s · 2 tool calls · 1 file changed · $0.03", Turn: 1},
		{Kind: api.EntryTurn, Who: "critic", Text: "turn failed", Error: true, Turn: 1},
		{Kind: api.EntryStatus, Who: "critic", Text: "error: claude: boom", Error: true},
		{Kind: api.EntryStatus, Who: "lead", Text: "stopped: killed", Error: true},
		{Kind: api.EntryModule, Who: "hikari", Text: "[[cancel]] orders set the part of orders to done (was testing)", Task: "cancel"},
		{Kind: api.EntryStep, Text: "Connected 3 agents"},
	}
	snap := s.snapshot()
	if len(snap.Feed) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(snap.Feed), len(want), snap.Feed)
	}
	for i, w := range want {
		got := snap.Feed[i]
		if got.ID != int64(i+1) || got.Time.IsZero() {
			t.Errorf("entry %d: id %d, time %v", i, got.ID, got.Time)
		}
		got.ID, got.Time = 0, w.Time
		if got != w {
			t.Errorf("entry %d:\n got %+v\nwant %+v", i, got, w)
		}
	}
	if snap.Cost != 0.75 {
		t.Errorf("cost = %v, want 0.75 (FD6)", snap.Cost)
	}

	var published []api.Entry
	for len(published) < len(want) {
		if e := <-events; e.Type == "feed" {
			published = append(published, *e.Entry)
		}
	}
	if published[3].Text != want[3].Text || published[8].Kind != api.EntryStep {
		t.Errorf("published entries differ: %+v", published)
	}
	lines := strings.Split(strings.TrimSpace(journal.String()), "\n")
	if len(lines) != len(want) {
		t.Fatalf("journal has %d lines, want %d", len(lines), len(want))
	}
	var first api.Entry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.Text != want[0].Text || first.Kind != "message" {
		t.Errorf("journal line 1 = %s (%v)", lines[0], err)
	}
}

func TestFeedKeepsTheLastEntries(t *testing.T) {
	s, _ := newTestServer(t)
	for range maxFeed + 10 {
		s.Step("x", nil)
	}
	feed := s.snapshot().Feed
	if len(feed) != maxFeed || feed[0].ID != 11 {
		t.Errorf("kept %d entries, first id %d", len(feed), feed[0].ID)
	}
}

func TestDuration(t *testing.T) {
	for ms, want := range map[int64]string{2500: "2.5s", 45000: "45s", 192000: "3m"} {
		if got := duration(ms); got != want {
			t.Errorf("duration(%d) = %s, want %s", ms, got, want)
		}
	}
}
