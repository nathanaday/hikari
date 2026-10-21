// Package node runs inside an agent container. It waits for messages from the
// hub and gives each batch to Claude Code as one headless turn, resuming the
// same session every time.
package node

import (
	"bufio"
	"bytes"
	"context"
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
	sys := systemPrompt(self)
	log.Printf("connected as %s; neighbors: %s", self.Name, peerNames(self.Neighbors))
	report(ctx, c, api.Status{State: api.StateIdle})

	var session string
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
		report(ctx, c, api.Status{State: api.StateWorking})
		t := turn{prompt: mcpserver.FormatMessages(msgs), session: session, model: self.Model, mcpConfig: mcpConfig, system: sys}
		res, err := t.run(ctx)
		if res.session != "" {
			session = res.session
		}
		if err != nil {
			log.Printf("turn failed: %v", err)
			report(ctx, c, api.Status{State: api.StateError, Detail: err.Error()})
			continue
		}
		log.Printf("turn done in %s, cost $%.4f", res.duration.Round(100*time.Millisecond), res.cost)
		report(ctx, c, api.Status{State: api.StateIdle})
	}
	return nil
}

func connect(ctx context.Context, c *api.Client) (api.Self, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		self, err := c.Self(ctx)
		if err == nil {
			return self, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return api.Self{}, fmt.Errorf("reach hub at %s: %w", c.URL, err)
		}
		sleep(ctx, time.Second)
	}
}

func report(ctx context.Context, c *api.Client, s api.Status) {
	if err := c.SetStatus(ctx, s); err != nil && ctx.Err() == nil {
		log.Printf("report status: %v", err)
	}
}

// writeMCPConfig points Claude Code at this same binary in MCP mode. The hub
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
			"env":     map[string]string{"HIKARI_HUB_URL": c.URL, "HIKARI_TOKEN": c.Token},
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
	b.WriteString("\n\nYou can reach only these neighbors:\n")
	for _, p := range self.Neighbors {
		fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Description)
	}
	b.WriteString(`
Rules:
- Messages from neighbors arrive as your prompt, inside <message> tags.
- Nobody reads your text output. To give information to a neighbor, call the mcp__hikari__send_message tool.
- You cannot reach nodes that are not your neighbors. If a task needs one, ask a neighbor to relay it.
- After you send what is needed, end your turn. A reply arrives as a new turn.
- During a long turn, call mcp__hikari__check_inbox to read messages that arrived meanwhile.
- Send a message only when it contains information or a request. Do not send messages that only acknowledge or thank.
`)
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

func peerNames(ps []api.Peer) string {
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = p.Name
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
