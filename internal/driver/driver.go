// Package driver runs inside an agent container. It waits for messages from the
// server and gives each batch to Claude Code as one headless turn, resuming the
// same session every time.
package driver

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
	shared, err := newSharedArea(self, deliverableNames(self), self.Skills)
	if err != nil {
		return err
	}
	d := &driver{c: c, self: self, mcpConfig: mcpConfig, shared: shared, tap: newTap(ctx, c)}
	log.Printf("connected as %s, %s", self.Name, roleOf(self))

	if self.Partition != nil {
		tc, err := c.TurnContext(ctx)
		if err != nil {
			log.Printf("turn context: %v", err)
		} else if tc.MapState == mapUnbuilt || tc.MapState == mapStale {
			log.Printf("indexing: the feature map is %s", tc.MapState)
			d.turn(ctx, api.StateIndexing, "indexing the partition: the map is "+tc.MapState, indexingPrompt(self, tc))
		} else if tc.NoLocalResults {
			log.Printf("restoring: this machine lacks the partition's results")
			d.turn(ctx, api.StateIndexing, "restoring the partition on this machine", restorePrompt(tc))
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
		d.turn(ctx, api.StateWorking, trigger(msgs), turnNotices(self, tc)+mcpserver.FormatMessages(msgs))
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
	tap       *tap
	session   string
}

// turn runs one Claude Code turn while the agent shows state. After the
// turn of a partition agent or the orchestration agent, it asks the server
// to commit what the turn left, even when the turn failed.
func (d *driver) turn(ctx context.Context, state, why, prompt string) {
	report(ctx, d.c, api.Status{State: state})
	d.tap.begin(why)
	// The vault can change between turns, so the skills and the policies
	// are connected again before each one.
	partition := systemPrompt(d.self) + d.shared.sync()
	t := turn{prompt: prompt, session: d.session, model: d.self.Model, mcpConfig: d.mcpConfig, partition: partition, tap: d.tap}
	res, err := t.run(ctx)
	d.tap.end(res, res.closing, err)
	if res.session != "" {
		d.session = res.session
	}
	if (d.self.Partition != nil || d.self.Orchestration) && ctx.Err() == nil {
		if commits, cerr := d.c.Commit(ctx, ""); cerr != nil {
			log.Printf("automatic apply: %v", cerr)
		} else if len(commits) > 0 {
			log.Printf("applied changes left by the turn: %s", strings.Join(commits, ", "))
		}
	}
	if err != nil {
		log.Printf("turn failed: %v", err)
		report(ctx, d.c, api.Status{State: api.StateError, Detail: err.Error()})
		return
	}
	log.Printf("turn done in %s, cost $%.4f", res.duration.Round(100*time.Millisecond), res.cost)
	report(ctx, d.c, api.Status{State: api.StateIdle, Cost: res.cost})
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
	b.WriteString(roomPrompt(self))
	b.WriteString(sharedPrompt(self))
	b.WriteString(wikisPrompt(self))
	if self.Partition != nil {
		b.WriteString(partitionAgentPrompt(self))
	}
	if self.Orchestration {
		b.WriteString(orchestrationPrompt(self))
	}
	b.WriteString(grantsPrompt(self))
	if self.Partition != nil {
		b.WriteString(deliverablesPrompt(self))
	}
	if self.Instructions != "" {
		fmt.Fprintf(&b, "\n# Instructions\n\n%s\n", self.Instructions)
	}
	return b.String()
}

type turn struct {
	prompt, session, model, mcpConfig, partition string
	tap                                          *tap
}

type turnResult struct {
	session  string
	duration time.Duration
	cost     float64
	// closing is the text that ended the turn. It reaches nobody unless
	// the agent sent it as a message.
	closing string
}

func (t turn) run(ctx context.Context) (turnResult, error) {
	args := []string{
		"-p",
		"--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--mcp-config", t.mcpConfig, "--strict-mcp-config",
		"--append-system-prompt", t.partition,
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
		if t.tap != nil {
			t.tap.observe(e)
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
		res.cost, res.closing = final.Cost, final.Result
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

// roleOf names the agent's role in the room, for the log.
func roleOf(self api.Self) string {
	if self.Orchestration {
		return "the orchestration agent"
	}
	return "a partition agent"
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
