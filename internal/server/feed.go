package server

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
)

// The feed: one entry per event that a person follows the run by (rules
// FD1 to FD3 of the feed spec). The server makes the entries, so that the
// terminal and the UI show the same ones.

// maxFeed is the most entries kept.
const maxFeed = 500

// FeedTo appends each later entry to w as one line of JSON (rule FD2).
func (s *Server) FeedTo(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feedJournal = w
}

// Step feeds a start-up step of hikari up with its result, or with the
// error that failed it (rule FD4).
func (s *Server) Step(result string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.feed(api.Entry{Kind: api.EntryStep, Text: result + ": " + err.Error(), Error: true})
		return
	}
	s.feed(api.Entry{Kind: api.EntryStep, Text: result})
}

// feed records e and publishes it. The caller holds mu.
func (s *Server) feed(e api.Entry) {
	s.nextEntry++
	e.ID = s.nextEntry
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	s.entries = append(s.entries, e)
	if len(s.entries) > maxFeed {
		s.entries = slices.Clone(s.entries[len(s.entries)-maxFeed:])
	}
	if s.feedJournal != nil {
		line, _ := json.Marshal(e)
		s.feedJournal.Write(append(line, '\n'))
	}
	s.publish(Event{Type: "feed", Entry: &e})
}

// statusEntry is the entry of a change of state, or nil when the change
// makes none: an agent connects, fails, or exits with a reason (decision
// 1).
func statusEntry(agent string, from, to api.Status) *api.Entry {
	switch {
	case to.State == api.StateIdle && from.State == api.StateStarting:
		return &api.Entry{Kind: api.EntryStatus, Who: agent, Text: "connected"}
	case to.State == api.StateError && (from.State != to.State || from.Detail != to.Detail):
		return &api.Entry{Kind: api.EntryStatus, Who: agent, Text: "error: " + to.Detail, Error: true}
	case to.State == api.StateExited && to.Detail != "" && (from.State != to.State || from.Detail != to.Detail):
		return &api.Entry{Kind: api.EntryStatus, Who: agent, Text: "stopped: " + to.Detail, Error: true}
	}
	return nil
}

var editTools = []string{"Edit", "MultiEdit", "Write", "NotebookEdit"}

// turnSummary describes an ended turn in one line, as the UI's turn card
// does: its duration, its tool calls, the files it changed, and its cost.
func turnSummary(steps []api.Activity, end api.Activity) string {
	tools := 0
	files := map[string]bool{}
	for _, a := range steps {
		if a.Kind != api.ActivityTool {
			continue
		}
		tools++
		if slices.Contains(editTools, a.Tool) {
			var in struct {
				FilePath string `json:"file_path"`
			}
			json.Unmarshal(a.Input, &in)
			files[in.FilePath] = true
		}
	}
	var parts []string
	if end.Duration > 0 {
		parts = append(parts, duration(end.Duration))
	}
	if tools > 0 {
		parts = append(parts, fmt.Sprintf("%d tool %s", tools, plural(tools, "call")))
	}
	if len(files) > 0 {
		parts = append(parts, fmt.Sprintf("%d %s changed", len(files), plural(len(files), "file")))
	}
	if end.Cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", end.Cost))
	}
	return strings.Join(parts, " · ")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// duration writes milliseconds as the UI does: tenths under ten seconds,
// seconds under ninety, minutes after.
func duration(ms int64) string {
	s := float64(ms) / 1000
	switch {
	case s < 10:
		return fmt.Sprintf("%.1fs", s)
	case s < 90:
		return fmt.Sprintf("%.0fs", s)
	}
	return fmt.Sprintf("%.0fm", s/60)
}
