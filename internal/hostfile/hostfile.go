// Package hostfile is .hikari/host.json beside a topology file: the address
// of the UI of a running hikari up, which hikari send and the commands of
// the modules call (rules CP11 and CP12 of the plugin spec).
package hostfile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Cookie carries the UI's token; it is the server's UICookie.
const Cookie = "hikari_ui"

// Host is the address of a running hikari up.
type Host struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// Path is the host file of a topology file.
func Path(file string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	return filepath.Join(filepath.Dir(abs), ".hikari", "host.json")
}

// Write writes the file readable only by you, since it holds the UI's
// token.
func Write(file string, h Host) error {
	p := Path(file)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(h, "", "  ")
	os.Remove(p)
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// Remove removes the file when this process wrote it.
func Remove(file string) {
	if h, err := Read(file); err == nil && h.PID == os.Getpid() {
		os.Remove(Path(file))
	}
}

// Read reads the host file of a topology file.
func Read(file string) (Host, error) {
	var h Host
	data, err := os.ReadFile(Path(file))
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(data, &h)
}

// Alive reports whether the process pid lives.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Live returns the host of a running hikari up for file.
func Live(file string) (Host, bool) {
	h, err := Read(file)
	return h, err == nil && Alive(h.PID)
}

// ErrNotRunning says that no hikari up runs for a topology file.
var ErrNotRunning = errors.New("hikari is not running")

// NotRunning is the error for file, with the command that starts hikari.
func NotRunning(file string, shown string) error {
	return fmt.Errorf("%w for %s; start it in your own terminal with: hikari up %s", ErrNotRunning, shown, shown)
}

// Call sends one request of the UI's API to the running hikari up for
// file, as you, and decodes its answer into out.
func Call(ctx context.Context, file, shown, method, path string, in, out any) error {
	h, err := Read(file)
	if errors.Is(err, iofs.ErrNotExist) || (err == nil && !Alive(h.PID)) {
		return NotRunning(file, shown)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", Path(file), err)
	}
	var body io.Reader
	if in != nil {
		data, _ := json.Marshal(in)
		body = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(h.URL, "/")+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: Cookie, Value: h.Token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return NotRunning(file, shown)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("the server answered %s", resp.Status)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
