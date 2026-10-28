// Command pin writes the launcher of the plugin's version, with the
// checksums of plugin/release/checksums.txt, to plugin/bin/hikari. With
// -set VERSION, it first sets the version in the plugin's manifest and in
// the marketplace entry (its version and its ref). Run it from the
// repository root (make pin, make version).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nathanaday/hikari/internal/release"
)

const (
	manifest    = "plugin/.claude-plugin/plugin.json"
	marketplace = ".claude-plugin/marketplace.json"
)

func main() {
	var err error
	if len(os.Args) == 3 && os.Args[1] == "-set" {
		err = setVersion(os.Args[2])
	} else {
		err = pin()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pin:", err)
		os.Exit(1)
	}
}

func pin() error {
	version, err := pluginVersion()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(release.ChecksumsFile)
	if err != nil {
		return err
	}
	sums, err := release.ParseChecksums(string(data))
	if err != nil {
		return err
	}
	if missing := release.Missing(version, sums); len(missing) > 0 {
		return fmt.Errorf("%s names no checksum for %v; run make release first", release.ChecksumsFile, missing)
	}
	if err := os.MkdirAll(filepath.Dir(release.LauncherFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(release.LauncherFile, []byte(release.Launcher(version, string(data))), 0o755)
}

// semver is a release version as its tag names it: no v in front.
var semver = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// setVersion replaces the plugin's version in every file that names it. The
// launcher and its checksums follow with make pin.
func setVersion(version string) error {
	if !semver.MatchString(version) {
		return fmt.Errorf("%q is not major.minor.patch, such as 0.2.0 (no v in front)", version)
	}
	old, err := pluginVersion()
	if err != nil {
		return err
	}
	edits := []struct{ file, from string }{
		{manifest, `"version": "` + old + `"`},
		{marketplace, `"version": "` + old + `"`},
		{marketplace, `"ref": "` + old + `"`},
	}
	for _, e := range edits {
		data, err := os.ReadFile(e.file)
		if err != nil {
			return err
		}
		text := string(data)
		if n := strings.Count(text, e.from); n != 1 {
			return fmt.Errorf("%s holds %s %d times, not once; set the version by hand", e.file, e.from, n)
		}
		to := strings.Replace(e.from, old, version, 1)
		if err := os.WriteFile(e.file, []byte(strings.Replace(text, e.from, to, 1)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func pluginVersion() (string, error) {
	var m struct {
		Version string `json:"version"`
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, &m); err != nil || m.Version == "" {
		return "", fmt.Errorf("%s holds no version: %v", manifest, err)
	}
	return m.Version, nil
}
