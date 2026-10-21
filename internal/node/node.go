// Package node runs inside an agent container. It waits for messages from the
// server and gives each batch to Claude Code as one headless turn, resuming the
// same session every time.
package node

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/mcpserver"
)

const pollWait = 30 * time.Second

func Run(ctx context.Context, c *api.Client) error {
	self, err := connect(ctx, c)
	if err != nil {
		return err
	}
	mcpConfig, err := writeMCPConfig(c)
	if err != nil {
		return err
	}
	shared, err := newSharedArea(self.System != nil)
	if err != nil {
		return err
	}
	d := &driver{c: c, self: self, mcpConfig: mcpConfig, shared: shared}
	log.Printf("connected as %s; hubs: %s", self.Name, hubNames(self.Hubs))

	if self.System != nil {
		tc, err := c.TurnContext(ctx)
		if err != nil {
			log.Printf("turn context: %v", err)
		} else if tc.MapState == mapUnbuilt || tc.MapState == mapStale {
			log.Printf("indexing: the system map is %s", tc.MapState)
			d.turn(ctx, api.StateIndexing, indexingPrompt(self, tc))
		}
	}
	report(ctx, c, api.Status{State: api.StateIdle})

	for ctx.Err() == nil {
		msgs, err := c.Inbox(ctx, pollWait)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("inbox: %v", err)
				sleep(ctx, 2*time.Second)
			}
			continue
		}
		if len(msgs) == 0 {
			continue
		}
		log.Printf("turn: %d message(s) from %s", len(msgs), senders(msgs))
		tc, err := c.TurnContext(ctx)
		if err != nil {
			log.Printf("turn context: %v", err)
		}
		d.turn(ctx, api.StateWorking, turnNotices(self, tc)+mcpserver.FormatMessages(msgs))
	}
	return nil
}

// Map states, as the server reports them.
const (
	mapUnbuilt = "unbuilt"
	mapStale   = "stale"
)

type driver struct {
	c         *api.Client
	self      api.Self
	mcpConfig string
	shared    *sharedArea
	session   string
}

// turn runs one Claude Code turn while the agent shows state. After an
// owner's turn, it asks the server to commit what the turn left, even when
// the turn failed.
func (d *driver) turn(ctx context.Context, state, prompt string) {
	report(ctx, d.c, api.Status{State: state})
	// The shared area can change between turns, so the skills, agents, and
	// policies are connected again before each one.
	system := systemPrompt(d.self) + d.shared.sync()
	t := turn{prompt: prompt, session: d.session, model: d.self.Model, mcpConfig: d.mcpConfig, system: system}
	res, err := t.run(ctx)
	if res.session != "" {
		d.session = res.session
	}
	if d.self.System != nil && ctx.Err() == nil {
		if commit, cerr := d.c.Commit(ctx, ""); cerr != nil {
			log.Printf("automatic commit: %v", cerr)
		} else if commit != "" {
			log.Printf("committed changes left by the turn: %s", commit)
		}
	}
	if err != nil {
		log.Printf("turn failed: %v", err)
		report(ctx, d.c, api.Status{State: api.StateError, Detail: err.Error()})
		return
	}
	log.Printf("turn done in %s, cost $%.4f", res.duration.Round(100*time.Millisecond), res.cost)
	report(ctx, d.c, api.Status{State: api.StateIdle})
}

func connect(ctx context.Context, c *api.Client) (api.Self, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		self, err := c.Self(ctx)
		if err == nil {
			return self, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return api.Self{}, fmt.Errorf("reach server at %s: %w", c.URL, err)
		}
		sleep(ctx, time.Second)
	}
}

func report(ctx context.Context, c *api.Client, s api.Status) {
	if err := c.SetStatus(ctx, s); err != nil && ctx.Err() == nil {
		log.Printf("report status: %v", err)
	}
}

// writeMCPConfig points Claude Code at this same binary in MCP mode. The server
// credentials go in the config because Claude Code does not pass its whole
// environment to MCP servers.
func writeMCPConfig(c *api.Client) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cfg := map[string]any{"mcpServers": map[string]any{
		"hikari": map[string]any{
			"command": exe,
			"args":    []string{"mcp"},
			"env":     map[string]string{"HIKARI_SERVER_URL": c.URL, "HIKARI_TOKEN": c.Token},
		},
	}}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	path := filepath.Join(os.TempDir(), "hikari-mcp.json")
	return path, os.WriteFile(path, data, 0o600)
}

func systemPrompt(self api.Self) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# hikari network\n\nYou are the agent %q in a hikari network of agents.", self.Name)
	if self.Description != "" {
		fmt.Fprintf(&b, " Your role: %s", self.Description)
	}
	fmt.Fprintf(&b, "\n\n## Your hubs\n\n%s", mcpserver.DescribeHubs(self))
	b.WriteString(`
## How messages work

- A hub is a chat room. mcp__hikari__post_to_hub sends a message to every participant of a hub.
- mcp__hikari__send_direct_message sends a private message to one node that shares a hub with you. You cannot reach any other node.
- Messages arrive as your prompt inside <message> tags. The channel attribute is the hub (#name) or "direct".
- Nobody reads your text output. Only the hikari tools deliver information.
- Answer in the channel the message came from: a direct message with a direct message, a hub post in that hub.
- After you send what is needed, end your turn. Replies arrive as a new turn.
- Send a message only when it contains information or a request. Do not send messages that only acknowledge or thank. If a message needs nothing from you, end your turn without sending anything.
- During a long turn, call mcp__hikari__check_inbox to read messages that arrived meanwhile.

## When a message starts a turn for you

Every participant of a hub receives every post. A post starts a turn at once only for the orchestrator of the hub, for each participant that the post mentions as @name, and for all members when the orchestrator posts without mentions. You read other posts at the start of your next turn. A direct message always starts a turn.

## Your role in each hub

`)
	for _, h := range self.Hubs {
		orchestrator := h.Participants[0].Name
		if orchestrator == self.Name {
			fmt.Fprintf(&b, "- In #%s you are the orchestrator. You receive every post at once. Coordinate the members: give each member a clear task with @name, collect the results, and report to the participant who asked. A post from you without mentions starts a turn for every member, so mention only the members you need.\n", h.Name)
		} else {
			fmt.Fprintf(&b, "- In #%s you are a member. %s coordinates the hub. Do the tasks that mention you and post the results in the hub. A post from you starts a turn for %s and for each participant you mention.\n", h.Name, orchestrator, orchestrator)
		}
	}
	b.WriteString(sharedPrompt())
	if self.System != nil {
		b.WriteString(ownerPrompt(self))
	}
	if len(self.Orchestrates) > 0 {
		b.WriteString(orchestratorPrompt(self))
	}
	b.WriteString(grantsPrompt(self))
	if self.Instructions != "" {
		fmt.Fprintf(&b, "\n# Instructions\n\n%s\n", self.Instructions)
	}
	return b.String()
}

type turn struct {
	prompt, session, model, mcpConfig, system string
}

type turnResult struct {
	session  string
	duration time.Duration
	cost     float64
}

// streamEvent holds the fields of Claude Code's stream-json output that the
// driver uses.
type streamEvent struct {
	Type      string  `json:"type"`
	SessionID string  `json:"session_id"`
	IsError   bool    `json:"is_error"`
	Result    string  `json:"result"`
	Cost      float64 `json:"total_cost_usd"`
	Message   struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
}

func (t turn) run(ctx context.Context) (turnResult, error) {
	args := []string{
		"-p",
		"--output-format", "stream-json", "--verbose",
		"--mcp-config", t.mcpConfig, "--strict-mcp-config",
		"--append-system-prompt", t.system,
		"--dangerously-skip-permissions",
	}
	if t.session != "" {
		args = append(args, "--resume", t.session)
	}
	if t.model != "" {
		args = append(args, "--model", t.model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(t.prompt)
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return turnResult{}, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return turnResult{}, err
	}

	var res turnResult
	var final *streamEvent
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var e streamEvent
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if e.SessionID != "" {
			res.session = e.SessionID
		}
		switch e.Type {
		case "assistant":
			for _, c := range e.Message.Content {
				switch c.Type {
				case "text":
					log.Printf("  text: %s", clip(c.Text, 300))
				case "tool_use":
					log.Printf("  tool: %s %s", c.Name, clip(string(c.Input), 300))
				}
			}
		case "result":
			final = &e
		}
	}
	waitErr := cmd.Wait()
	res.duration = time.Since(start)

	switch {
	case final != nil && final.IsError:
		return res, fmt.Errorf("claude: %s", clip(final.Result, 500))
	case final != nil:
		res.cost = final.Cost
		return res, nil
	case waitErr != nil:
		return res, fmt.Errorf("claude: %v: %s", waitErr, clip(stderr.String(), 500))
	default:
		return res, fmt.Errorf("claude exited without a result")
	}
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func hubNames(hubs []api.HubInfo) string {
	names := make([]string, len(hubs))
	for i, h := range hubs {
		names[i] = "#" + h.Name
	}
	return strings.Join(names, ", ")
}

func senders(msgs []api.Message) string {
	var names []string
	seen := map[string]bool{}
	for _, m := range msgs {
		if !seen[m.From] {
			seen[m.From] = true
			names = append(names, m.From)
		}
	}
	return strings.Join(names, ", ")
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
