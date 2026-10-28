package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/hostfile"
	"github.com/nathanaday/hikari/internal/scaffold"
	"github.com/nathanaday/hikari/internal/version"
)

// hook runs a hook of the Hikari plugin. A hook never fails a session, so
// every error ends in silence.
func hook(args []string) error {
	if len(args) == 0 || args[0] != "session-start" {
		return nil
	}
	var in struct {
		Cwd string `json:"cwd"`
	}
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice == 0 {
		json.NewDecoder(os.Stdin).Decode(&in)
	}
	if in.Cwd == "" {
		in.Cwd, _ = os.Getwd()
	}
	fmt.Print(sessionStart(in.Cwd))
	return nil
}

// sessionStart is the context of a session that starts in dir (rule CP14
// of the plugin spec): nothing outside a hikari project, and a few lines in
// one.
func sessionStart(dir string) string {
	file := filepath.Join(dir, scaffold.TopologyFile)
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var top struct {
		Name   string    `yaml:"name"`
		Hikari yaml.Node `yaml:"x-hikari"`
	}
	if yaml.Unmarshal(data, &top) != nil || top.Hikari.Kind == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "This folder is the hikari project %s, with hikari %s. The hikari skill routes requests about it.\n", top.Name, version.Version)
	if stage := stageOf(filepath.Join(dir, "onboarding", "state.md")); stage != "" {
		fmt.Fprintf(&b, "Onboarding stage: %s.\n", stage)
	}
	changes, _ := filepath.Glob(filepath.Join(dir, "changes", "*", "state.md"))
	var open []string
	for _, c := range changes {
		if stage := stageOf(c); stage != "" && stage != "change: done" {
			open = append(open, fmt.Sprintf("%s (%s)", filepath.Base(filepath.Dir(c)), stage))
		}
	}
	if len(open) > 0 {
		fmt.Fprintf(&b, "Open topology changes in changes/: %s.\n", strings.Join(open, ", "))
	}
	if h, ok := hostfile.Live(file); ok {
		fmt.Fprintf(&b, "hikari runs for it, with its UI at %s; post to its room with hikari send.\n", h.URL)
	} else {
		b.WriteString("hikari is not running for it.\n")
	}
	return b.String()
}

// stageOf reads the Stage line of a state file.
func stageOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, _ := io.ReadAll(io.LimitReader(f, 64<<10))
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Stage:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}
