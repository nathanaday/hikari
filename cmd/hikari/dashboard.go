package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/server"
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
	con.Println("  " + con.Dim("Claude Code agents in containers, talking through hubs"))
	con.Println("")
}

// dashboard follows the server's events. It prints each message as one line
// and keeps the footer: the web UI address, each agent's state, and the cost
// so far.
type dashboard struct {
	con     *console.Console
	url     string
	logPath string
	agents  []string

	mu        sync.Mutex
	status    map[string]api.Status
	cost      float64
	connected chan struct{} // closed when no agent is starting
}

func newDashboard(con *console.Console, srv *server.Server, agents []string, url, logPath string) *dashboard {
	d := &dashboard{con: con, url: url, logPath: logPath, agents: agents,
		status: map[string]api.Status{}, connected: make(chan struct{})}
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
			d.setStatus(e.Node, *e.Status)
		case "message":
			d.con.Println(d.messageLine(*e.Message))
		}
		d.con.Refresh()
	}
}

func (d *dashboard) setStatus(node string, s api.Status) {
	d.mu.Lock()
	d.status[node] = s
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

	switch {
	case s.State == api.StateError:
		d.con.Error(fmt.Sprintf("%s: %s", node, s.Detail))
	case s.State == api.StateExited && s.Detail != "":
		d.con.Error(fmt.Sprintf("%s stopped: %s", node, s.Detail))
	}
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

func (d *dashboard) messageLine(m api.Message) string {
	to := "#" + m.Hub
	if m.Hub == "" {
		to = m.To
	}
	body := strings.Join(strings.Fields(m.Body), " ")
	line := fmt.Sprintf("  %s  %s → %s  %s", d.con.Dim(m.Time.Local().Format("15:04")), d.con.Bold(m.From), to, body)
	// One line per message; the web UI shows the whole text.
	return console.Truncate(line, d.con.Width()-1)
}

// footer returns the pinned lines.
func (d *dashboard) footer(frame int) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := d.con
	var agents []string
	for _, a := range d.agents {
		agents = append(agents, d.glyph(d.status[a].State, frame)+" "+a)
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
