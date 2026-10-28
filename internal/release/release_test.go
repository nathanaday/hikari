package release

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	sum := strings.Repeat("a", 64)
	got, err := ParseChecksums(sum + "  hikari-0.1.0-linux-amd64\n\n")
	if err != nil || got["hikari-0.1.0-linux-amd64"] != sum {
		t.Fatalf("ParseChecksums = %v, %v", got, err)
	}
	if _, err := ParseChecksums("abc  hikari\n"); err == nil {
		t.Error("a short sha256 is refused")
	}
	if m := Missing("0.1.0", got); len(m) != 3 || m[0] != "hikari-0.1.0-darwin-amd64" {
		t.Errorf("Missing = %v", m)
	}
}

// release makes a fake release of version in dir: each asset is a script
// that prints its own name and its arguments. It returns the checksums.
func fakeRelease(t *testing.T, dir, version string) string {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, version), 0o755)
	var sums strings.Builder
	for _, p := range Platforms {
		asset := Asset(version, p)
		data := []byte(fmt.Sprintf("#!/bin/sh\necho %s \"$@\"\n", asset))
		os.WriteFile(filepath.Join(dir, version, asset), data, 0o755)
		h := sha256.Sum256(data)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(h[:]), asset)
	}
	return sums.String()
}

func host() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func runLauncher(t *testing.T, launcher, home, base, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{launcher}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HIKARI_HOME="+home, "HIKARI_RELEASE_BASE="+base, "HIKARI_BIN=", "HIKARI_NO_DOWNLOAD=")
	cmd.Stdin = strings.NewReader("{}")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestLauncher checks CP4 and CP5 of the plugin spec.
func TestLauncher(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no release for", runtime.GOOS)
	}
	rel := t.TempDir()
	sums := fakeRelease(t, rel, "9.9.9")
	launcher := filepath.Join(t.TempDir(), "hikari")
	os.WriteFile(launcher, []byte(Launcher("9.9.9", sums)), 0o755)
	home, work := t.TempDir(), t.TempDir()
	base := "file://" + rel

	// A hook downloads nothing; in a project it says what is missing.
	out, err := runLauncher(t, launcher, home, base, work, "hook", "session-start")
	if err != nil || out != "" {
		t.Errorf("CP5 a hook outside a project prints nothing: %q, %v", out, err)
	}
	os.WriteFile(filepath.Join(work, "topology.yaml"), []byte("name: x\n"), 0o644)
	out, err = runLauncher(t, launcher, home, base, work, "hook", "session-start")
	if err != nil || !strings.Contains(out, "not installed yet") || !strings.Contains(out, "the hikari skill installs it") {
		t.Errorf("CP5 a hook in a project says hikari is not installed: %q, %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(home, "bin")); err == nil {
		t.Error("CP5 a hook downloads nothing")
	}

	out, err = runLauncher(t, launcher, home, base, work, "version")
	want := Asset("9.9.9", host()) + " version"
	if err != nil || !strings.Contains(out, want) {
		t.Fatalf("CP4 the launcher installs and runs the host's binary: %q, %v", out, err)
	}
	dir := filepath.Join(home, "bin", "9.9.9")
	for _, a := range []string{"arm64", "amd64"} {
		_, err := os.Stat(filepath.Join(dir, "hikari-linux-"+a))
		if runtime.GOOS == "linux" && a == runtime.GOARCH {
			if err == nil {
				t.Errorf("on linux/%s, hikari is its own Linux binary", a)
			}
		} else if err != nil {
			t.Errorf("CP4 hikari-linux-%s is installed beside hikari: %v", a, err)
		}
	}
	if link, err := os.Readlink(filepath.Join(home, "bin", "hikari")); err != nil || link != "9.9.9/hikari" {
		t.Errorf("CP4 ~/.hikari/bin/hikari points at the new binary: %q, %v", link, err)
	}
	if log, _ := os.ReadFile(filepath.Join(home, "install.log")); !strings.Contains(string(log), "installed hikari 9.9.9") {
		t.Errorf("CP4 the install is logged: %q", log)
	}
	// The second run uses the installed binary, with no release to download from.
	if out, err := runLauncher(t, launcher, home, "file:///nowhere", work, "status"); err != nil || !strings.Contains(out, " status") {
		t.Errorf("CP4 an installed binary runs with no download: %q, %v", out, err)
	}

	// A file whose sha256 differs installs nothing.
	home2 := t.TempDir()
	os.WriteFile(filepath.Join(rel, "9.9.9", Asset("9.9.9", host())), []byte("#!/bin/sh\necho tampered\n"), 0o755)
	out, err = runLauncher(t, launcher, home2, base, work, "version")
	if err == nil || !strings.Contains(out, "nothing was installed") {
		t.Errorf("CP4 a mismatch is refused: %q, %v", out, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(home2, "bin", "9.9.9")); len(entries) != 0 {
		t.Errorf("CP4 a mismatch installs nothing: %v", entries)
	}
}

// TestPinnedLauncher checks that the plugin's launcher is the template with
// the pinned checksums, so an edit of either reaches the other by make pin.
func TestPinnedLauncher(t *testing.T) {
	sums, err := os.ReadFile("../../" + ChecksumsFile)
	if err != nil {
		t.Skip("no pinned release yet:", err)
	}
	got, err := os.ReadFile("../../" + LauncherFile)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimPrefix(strings.Fields(string(sums))[1], "hikari-")
	version = version[:strings.Index(version, "-")]
	if string(got) != Launcher(version, string(sums)) {
		t.Errorf("%s is not the launcher of %s; run make pin", LauncherFile, ChecksumsFile)
	}
}
