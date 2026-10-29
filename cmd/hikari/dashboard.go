package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/version"
)

// banner holds the letters of "hikari", two rows each.
var banner = [][2]string{
	{"█ █", "█▀█"},
	{"█", "█"},
	{"█▄▀", "█ █"},
	{"▄▀█", "█▀█"},
	{"█▀█", "█▀▄"},
	{"█", "█"},
}

// bannerColors run from amber to pale yellow, one color per letter.
var bannerColors = []int{208, 214, 220, 221, 228, 230}

func printBanner(con *console.Console) {
	con.Println("")
	for row := range 2 {
		var letters []string
		for i, l := range banner {
			letters = append(letters, con.Color256(bannerColors[i], l[row]))
		}
		con.Println("  " + strings.Join(letters, " "))
	}
	con.Println("  " + con.Dim("v"+version.Version))
	con.Println("")
}

// dashboard follows the server's events. It prints each entry of the feed
// as one line (rule FD7 of the feed spec) and keeps the footer: the web UI
// address, each agent's state, and the cost so far.
type dashboard struct {
	con     *console.Console
	url     string
	logPath string
	agents  []string

	mu        sync.Mutex
	status    map[string]api.Status
	doing     map[string]string // the current step of each working agent
	cost      float64
	connected chan struct{} // closed when no agent is starting
}

func newDashboard(con *console.Console, srv *server.Server, agents []string, url, logPath string) *dashboard {
	d := &dashboard{con: con, url: url, logPath: logPath, agents: agents,
		status: map[string]api.Status{}, doing: map[string]string{}, connected: make(chan struct{})}
	events, _ := srv.Subscribe()
	go d.follow(events)
	return d
}

func (d *dashboard) follow(events <-chan server.Event) {
	for e := range events {
		switch e.Type {
		case "snapshot":
			d.mu.Lock()
			for _, a := range e.Snapshot.Agents {
				d.status[a.Name] = a.Status
			}
			d.mu.Unlock()
		case "status":
			d.setStatus(e.Agent, *e.Status)
		case "feed":
			// A step was printed as it ended (decision 2).
			if e.Entry.Kind != api.EntryStep {
				d.con.Println(d.entryLine(*e.Entry))
			}
		case "activity":
			d.setDoing(e.Agent, *e.Activity)
		}
		d.con.Refresh()
	}
}

func (d *dashboard) setStatus(agent string, s api.Status) {
	d.mu.Lock()
	d.status[agent] = s
	d.cost += s.Cost
	starting := false
	for _, a := range d.agents {
		starting = starting || d.status[a].State == api.StateStarting
	}
	if !starting {
		select {
		case <-d.connected:
		default:
			close(d.connected)
		}
	}
	d.mu.Unlock()
}

// setDoing keeps the step that the footer shows for an agent: its running
// tool, or nothing between tools and after the turn.
func (d *dashboard) setDoing(agent string, a api.Activity) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case a.Kind == api.ActivityTool && !a.Done:
		d.doing[agent] = toolSummary(a)
	case a.Kind == api.ActivityThinking && !a.Done:
		d.doing[agent] = "thinking"
	default:
		delete(d.doing, agent)
	}
}

// toolSummary names a tool call in a few words.
func toolSummary(a api.Activity) string {
	var in struct {
		FilePath string `json:"file_path"`
		Command  string `json:"command"`
		Pattern  string `json:"pattern"`
		Skill    string `json:"skill"`
	}
	json.Unmarshal(a.Input, &in)
	name := strings.TrimPrefix(a.Tool, "mcp__hikari__")
	switch {
	case in.FilePath != "":
		return name + " " + filepath.Base(in.FilePath)
	case in.Command != "":
		return name + " " + in.Command
	case in.Pattern != "":
		return name + " " + in.Pattern
	case in.Skill != "":
		return name + " " + in.Skill
	}
	return name
}

// waitConnected waits until every agent has reported, and reports whether
// they all did before the timeout.
func (d *dashboard) waitConnected(ctx context.Context, timeout time.Duration) bool {
	select {
	case <-d.connected:
		return true
	case <-time.After(timeout):
	case <-ctx.Done():
	}
	return false
}

// entryLine writes an entry as one line; the web UI shows the whole text.
func (d *dashboard) entryLine(e api.Entry) string {
	c := d.con
	who := c.Bold(e.Who)
	if e.Kind == api.EntryMessage {
		to := "room"
		if e.To != "" {
			to = e.To
		}
		who += " → " + to
	}
	text := strings.Join(strings.Fields(e.Text), " ")
	if e.Error {
		text = c.Red(text)
	}
	line := fmt.Sprintf("  %s  %s  %s", c.Dim(e.Time.Local().Format("15:04")), who, text)
	return console.Truncate(line, c.Width()-1)
}

// footer returns the pinned lines.
func (d *dashboard) footer(frame int) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := d.con
	var agents []string
	for _, a := range d.agents {
		label := a
		if doing := d.doing[a]; doing != "" {
			label += c.Dim(" · " + console.Truncate(doing, 32))
		}
		agents = append(agents, d.glyph(d.status[a].State, frame)+" "+label)
	}
	return []string{
		"",
		"  " + c.Bold("Web UI") + "  " + c.Link(d.url),
		"  " + c.Bold("Agents") + "  " + strings.Join(agents, "   "),
		"  " + c.Dim(fmt.Sprintf("$%.2f spent · log %s · Ctrl-C to stop", d.cost, d.logPath)),
	}
}

func (d *dashboard) glyph(state string, frame int) string {
	c := d.con
	switch state {
	case api.StateIdle:
		return c.Green("●")
	case api.StateWorking:
		return c.Yellow(console.Spinner(frame))
	case api.StateIndexing:
		return c.Blue(console.Spinner(frame))
	case api.StateError:
		return c.Red("✗")
	case api.StateExited:
		return c.Dim("■")
	default:
		return c.Dim("○")
	}
}

// lazyFile creates its file on the first write, so that a run without
// messages leaves no empty journal.
type lazyFile struct {
	path string
	mu   sync.Mutex
	f    *os.File
}

func (l *lazyFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		f, err := os.Create(l.path)
		if err != nil {
			return 0, err
		}
		l.f = f
	}
	return l.f.Write(p)
}

func (l *lazyFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	return l.f.Close()
}
