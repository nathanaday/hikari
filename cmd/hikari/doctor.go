package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/nathanaday/hikari/internal/agentimage"
	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/launcher"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/version"
)

// checkup collects the lines of hikari doctor (rule CP10 of the plugin
// spec). A problem blocks hikari up; a warning does not.
type checkup struct {
	con      *console.Console
	problems int
}

func (c *checkup) ok(msg string)   { c.con.Println("  " + c.con.Green("ok") + " " + msg) }
func (c *checkup) warn(msg string) { c.con.Warn(msg) }
func (c *checkup) fail(msg string) { c.problems++; c.con.Error(msg) }

func doctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	file, err := topologyArg(fs, args, "hikari doctor [topology.yaml]")
	if err != nil {
		return err
	}
	c := &checkup{con: console.New(os.Stdout)}
	c.binaries()
	c.claude()
	c.path()
	docker := c.docker(ctx)
	if docker {
		c.image(ctx)
	}
	c.git(ctx)
	if fs.NArg() == 1 || exists(file) {
		c.project(ctx, file)
	}
	if c.problems > 0 {
		c.con.Println("")
		c.con.Println(fmt.Sprintf("  %s to fix before hikari up", plural(c.problems, "problem")))
		return errShown
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hikariHome is the machine folder of hikari: $HIKARI_HOME, or ~/.hikari.
func hikariHome() string {
	if h := os.Getenv("HIKARI_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hikari")
}

func (c *checkup) binaries() {
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		c.fail("cannot find this binary: " + err.Error())
		return
	}
	c.ok(fmt.Sprintf("hikari %s in %s", version.Version, filepath.Dir(self)))
	var missing []string
	for _, arch := range []string{"arm64", "amd64"} {
		if _, err := launcher.LinuxBinary(arch); err != nil {
			missing = append(missing, "hikari-linux-"+arch)
		}
	}
	if len(missing) > 0 {
		c.fail(fmt.Sprintf("%s missing beside it, which hikari mounts into the containers; install hikari again with the plugin (remove %s and run hikari) or with make install", strings.Join(missing, " and "), filepath.Dir(self)))
	} else {
		c.ok("the Linux binaries for the containers beside it")
	}
}

// claude checks for Claude Code on the PATH, which the hikari agent in the
// web UI runs (rule HA11 of the hikari agent spec).
func (c *checkup) claude() {
	path, err := exec.LookPath("claude")
	if err != nil {
		c.warn("claude is not on the PATH; the hikari agent in the web UI needs Claude Code, and the containers bring their own")
		return
	}
	c.ok("claude in " + filepath.Dir(path))
}

// pathEntry reports whether bindir is on the PATH of new shells, from the
// entry files that install.sh writes or from PATH, and otherwise gives the
// command that writes the entry.
func pathEntry(bindir, goos, path string, read func(string) ([]byte, error)) (bool, string) {
	if slices.Contains(filepath.SplitList(path), bindir) {
		return true, ""
	}
	if goos == "darwin" {
		files, _ := filepath.Glob("/etc/paths.d/*")
		for _, f := range append([]string{"/etc/paths"}, files...) {
			data, _ := read(f)
			if slices.Contains(strings.Fields(string(data)), bindir) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("echo %s | sudo tee /etc/paths.d/hikari", bindir)
	}
	data, _ := read("/etc/profile.d/hikari.sh")
	if strings.Contains(string(data), bindir) {
		return true, ""
	}
	return false, fmt.Sprintf(`echo 'export PATH="%s:$PATH"' | sudo tee /etc/profile.d/hikari.sh`, bindir)
}

func (c *checkup) path() {
	bindir := filepath.Join(hikariHome(), "bin")
	if !exists(filepath.Join(bindir, "hikari")) {
		c.warn(fmt.Sprintf("hikari is not installed in %s; the Hikari plugin installs it on first use, and make install installs a checkout", bindir))
	} else if ok, fix := pathEntry(bindir, runtime.GOOS, os.Getenv("PATH"), os.ReadFile); ok {
		c.ok(bindir + " on the PATH of your terminal")
	} else {
		c.warn(fmt.Sprintf("%s is not on the PATH of your terminal, where you run hikari up; add it once with: %s", bindir, fix))
	}
	if exists("/opt/hikari/bin") {
		c.warn("an older install is in /opt/hikari; remove it with: sudo rm -rf /opt/hikari")
	}
	found, err := exec.LookPath("hikari")
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, found, "version").Output()
	if got := strings.TrimSpace(string(out)); err != nil || got != "hikari "+version.Version {
		c.fail(fmt.Sprintf("the hikari that the PATH finds first, %s, is %q, not hikari %s; remove it, or put %s first", found, got, version.Version, bindir))
	}
}

func (c *checkup) docker(ctx context.Context) bool {
	if _, err := exec.LookPath("docker"); err != nil {
		c.fail("docker is not installed; install Docker Desktop, or Docker Engine with the compose plugin")
		return false
	}
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").CombinedOutput()
	if err != nil {
		c.fail("Docker does not answer; start Docker Desktop, or the Docker daemon: " + lastLine(out))
		return false
	}
	compose, err := exec.CommandContext(ctx, "docker", "compose", "version", "--short").CombinedOutput()
	if err != nil {
		c.fail("docker compose is missing; install the Docker Compose plugin: " + lastLine(compose))
		return false
	}
	c.ok(fmt.Sprintf("Docker %s, with docker compose %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(compose))))
	return true
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
}

func (c *checkup) image(ctx context.Context) {
	s, err := agentimage.Check(ctx)
	switch {
	case err != nil:
		c.warn(err.Error())
	case s == agentimage.Current:
		c.ok("the " + agentimage.Name + " image")
	case s == agentimage.Missing:
		c.warn(fmt.Sprintf("the %s image is not built; hikari up builds it, or run: hikari image", agentimage.Name))
	default:
		c.warn(fmt.Sprintf("the %s image comes from an older Dockerfile; hikari up builds it again, or run: hikari image", agentimage.Name))
	}
}

func (c *checkup) git(ctx context.Context) {
	get := func(key string) string {
		out, _ := exec.CommandContext(ctx, "git", "config", key).Output()
		return strings.TrimSpace(string(out))
	}
	name, email := get("user.name"), get("user.email")
	if name == "" || email == "" {
		c.warn("git has no user.name or user.email; the skills commit the project as you, so set them with git config --global")
		return
	}
	c.ok(fmt.Sprintf("git commits as %s <%s>", name, email))
}

func (c *checkup) project(ctx context.Context, file string) {
	topo, err := topology.Load(ctx, file)
	if err != nil {
		c.fail(fmt.Sprintf("%s has errors; run hikari check %s", linked.Shown(file), linked.Shown(file)))
		return
	}
	var without []string
	for _, n := range topo.AgentNames() {
		if !topo.ServiceOf(n).Credentials() {
			without = append(without, n)
		}
	}
	env := filepath.Join(filepath.Dir(topo.File), ".env")
	if len(without) > 0 {
		c.fail(fmt.Sprintf("no credential for %s; set CLAUDE_CODE_OAUTH_TOKEN in %s (make one with claude setup-token in your own terminal), or ANTHROPIC_API_KEY", strings.Join(without, ", "), linked.Shown(env)))
		return
	}
	c.ok(fmt.Sprintf("%s: a credential for each agent", linked.Shown(file)))
}
