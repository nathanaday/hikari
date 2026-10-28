package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/scaffold"
	"github.com/nathanaday/hikari/internal/server"
)

// hostFile is .hikari/host.json beside a topology file: the address of the
// UI of a running hikari up, which hikari send posts through (rules CP11
// and CP12 of the plugin spec).
type hostFile struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func hostPath(file string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	return filepath.Join(filepath.Dir(abs), ".hikari", "host.json")
}

// writeHost writes the file readable only by you, since it holds the UI's
// token.
func writeHost(file string, h hostFile) error {
	p := hostPath(file)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(h, "", "  ")
	os.Remove(p)
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// removeHost removes the file when this process wrote it.
func removeHost(file string) {
	if h, err := readHost(file); err == nil && h.PID == os.Getpid() {
		os.Remove(hostPath(file))
	}
}

func readHost(file string) (hostFile, error) {
	var h hostFile
	data, err := os.ReadFile(hostPath(file))
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(data, &h)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// liveHost returns the host of a running hikari up for file.
func liveHost(file string) (hostFile, bool) {
	h, err := readHost(file)
	return h, err == nil && processAlive(h.PID)
}

func notRunning(file string) error {
	return fmt.Errorf("hikari is not running for %s; start it in your own terminal with: hikari up %s", linked.Shown(file), linked.Shown(file))
}

func send(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	to := fs.String("to", "", "the agent to send a direct message to; without it, the message goes to the room")
	file := fs.String("file", scaffold.TopologyFile, "the topology file of the running project")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hikari send [-to <agent>] [-file topology.yaml] <message>, or - to read it from stdin")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	body := strings.Join(fs.Args(), " ")
	if body == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(data)
	}
	if strings.TrimSpace(body) == "" {
		fs.Usage()
		os.Exit(2)
	}
	m, err := sendMessage(ctx, *file, *to, body)
	if err != nil {
		return err
	}
	where := "the room"
	if m.To != "" {
		where = m.To
	}
	fmt.Printf("Sent to %s at %s.\n", where, m.Time.Local().Format("15:04:05"))
	return nil
}

// sendMessage posts body as you, to the room or to one agent, through the
// server of the running hikari up for file.
func sendMessage(ctx context.Context, file, to, body string) (api.Message, error) {
	var m api.Message
	h, err := readHost(file)
	if errors.Is(err, iofs.ErrNotExist) || (err == nil && !processAlive(h.PID)) {
		return m, notRunning(file)
	}
	if err != nil {
		return m, fmt.Errorf("%s: %w", hostPath(file), err)
	}
	data, _ := json.Marshal(api.SendRequest{To: to, Body: body})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(h.URL, "/")+"/api/ui/send", bytes.NewReader(data))
	if err != nil {
		return m, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: server.UICookie, Value: h.Token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return m, notRunning(file)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(out, &e) == nil && e.Error != "" {
			return m, errors.New(e.Error)
		}
		return m, fmt.Errorf("the server answered %s", resp.Status)
	}
	return m, json.Unmarshal(out, &m)
}
