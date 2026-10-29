package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/agentimage"
	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/driver"
	"github.com/nathanaday/hikari/internal/editor"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/hostfile"
	"github.com/nathanaday/hikari/internal/launcher"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/mcpserver"
	"github.com/nathanaday/hikari/internal/modules"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/version"
)

const usage = `hikari runs Claude Code agents in containers that talk to each other only through one room.

Usage:
  hikari up [flags] [topology.yaml]      start the server, the web UI, and the compose project
  hikari init [flags] <dir>              make a new project repository
  hikari sync [flags] [topology.yaml]    clone the linked repos, and make your branches from the remote
  hikari check [flags] [topology.yaml]   run the checks of hikari up, and start nothing
  hikari push [topology.yaml]            push your branches of the linked repos and the project
  hikari status [flags] [topology.yaml]  show each partition's feature map and recorded tests
  hikari send [flags] <message>          post a message to the room of the running project
  hikari doctor [topology.yaml]          check this machine, and the project, for hikari up
  hikari image                           build the hikari-agent image
  hikari task <command>                  plant stubs, list the pipeline, set statuses (the tasks module)
  hikari queue <command>                 make, arrange, start, and pause queues (the tasks module)
  hikari version                         print the version

Without a file, each command opens topology.yaml in the current directory.

Internal commands (hikari starts these; you do not run them):
  hikari driver   the agent driver; each container runs it
  hikari mcp      the agent's MCP tools; Claude Code starts it on each turn
  hikari hook     the hooks of the Hikari plugin for Claude Code
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "up":
		err = up(ctx, os.Args[2:])
	case "init":
		err = initProject(os.Args[2:])
	case "sync":
		err = syncProject(ctx, os.Args[2:])
	case "check":
		err = check(ctx, os.Args[2:])
	case "push":
		err = push(ctx, os.Args[2:])
	case "status":
		err = status(ctx, os.Args[2:])
	case "send":
		err = send(ctx, os.Args[2:])
	case "doctor":
		err = doctor(ctx, os.Args[2:])
	case "image":
		err = buildImage(ctx, os.Args[2:])
	case "hook":
		err = hook(os.Args[2:])
	case "driver":
		err = runDriver(ctx)
	case "mcp":
		err = runMCP(ctx)
	case "version", "-v", "-version", "--version":
		fmt.Println("hikari " + version.Version)
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
	default:
		cmd, ok := modules.Command(os.Args[1])
		if !ok {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = cmd(ctx, os.Args[2:])
	}
	if errors.Is(err, errShown) {
		os.Exit(1)
	}
	if err != nil {
		// Not log.Fatal: hikari up sends the log to its run's log file.
		fmt.Fprintf(os.Stderr, "hikari: %v\n", err)
		os.Exit(1)
	}
}

// errShown is an error that the console already showed.
var errShown = errors.New("shown")

func up(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("up", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:7420", "server address")
	noLaunch := fs.Bool("no-launch", false, "start only the server and print agent credentials, for running agents by hand")
	agent := fs.String("agent", "auto", "run the hikari agent in the web UI: auto (on for a loopback listener), on, or off")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hikari up [flags] [topology.yaml]")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	file, err := fileArg(fs)
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid -listen: %w", err)
	}
	ed, err := editor.New(file)
	if err != nil {
		return err
	}
	log.SetOutput(editLog())
	con := console.New(os.Stdout)
	printBanner(con)

	// Listen first, so that a busy port stops hikari before it touches any
	// repo, and so that the UI serves a broken file too (rule UP2).
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Print(err)
		con.Error(fmt.Sprintf("cannot listen on %s; pass another address with -listen (%v)", *listen, err))
		return errShown
	}
	uiToken := server.NewToken()
	host := server.NewHost(uiToken, ed)
	httpSrv := &http.Server{Handler: host.Handler()}
	go httpSrv.Serve(ln)
	defer httpSrv.Close()
	url := fmt.Sprintf("http://%s/?token=%s", ln.Addr(), uiToken)
	con.Println("  Web UI  " + con.Link(url))
	if err := hostfile.Write(ed.File, hostfile.Host{URL: "http://" + ln.Addr().String(), Token: uiToken, PID: os.Getpid()}); err != nil {
		log.Print(err)
		con.Warn("hikari send cannot reach this run: " + err.Error())
	}
	defer hostfile.Remove(ed.File)
	con.Println("")
	go ed.Watch(ctx, host.TopologyChanged)

	if *agent != "auto" && *agent != "on" && *agent != "off" {
		return fmt.Errorf("invalid -agent %q: use auto, on, or off", *agent)
	}
	r := &runner{ctx: ctx, con: con, file: ed.File, listen: *listen, port: port, noLaunch: *noLaunch, url: url, host: host, agent: *agent}
	host.Start = r.start
	host.Stop = func() { r.stop(false) }
	host.Starting()
	r.start()
	<-ctx.Done()
	log.Print("shutting down")
	r.stop(true)
	return nil
}

// prepareProject prepares the project repository before every other repo
// (rule PR8), and reads the topology again when the checkout of the project
// branch changed HEAD (rule PR9).
func prepareProject(ctx context.Context, topo *topology.Topology) (*topology.Topology, error) {
	p := topo.Project
	if p == nil {
		return topo, nil
	}
	repo := gitrepo.Open(p.Path)
	// Your edits in the wikis while hikari was stopped are knowledge, not
	// configuration, so hikari commits them as yours (rule C5 of the wikis
	// spec).
	if repo.Branch() == p.Branch {
		if changed, err := repo.Changed(); err == nil && len(changed) > 0 && !slices.ContainsFunc(changed, func(path string) bool { return !userPath(topo, path) }) {
			if _, err := repo.CommitPaths(p.Branch, changed, "", "user: wiki edits made while hikari was stopped"); err != nil {
				return nil, fmt.Errorf("the project repository: commit your wiki edits: %w", err)
			}
		}
	}
	before, _ := repo.Head()
	after, err := repo.Prepare(p.Branch)
	if err != nil {
		return nil, fmt.Errorf("the project repository: %w", err)
	}
	if after == before {
		return topo, nil
	}
	again, err := topology.Load(ctx, topo.File)
	if err != nil {
		return nil, fmt.Errorf("the topology file on the project branch %s: %w", p.Branch, err)
	}
	if again.Project == nil || again.Project.Branch != p.Branch {
		return nil, fmt.Errorf("the topology file on the project branch %s names another project branch, or none; make the branches agree", p.Branch)
	}
	if len(again.Missing) > 0 {
		return nil, fmt.Errorf("the topology file on the project branch %s links repos that are not cloned (%s); run hikari sync %s", p.Branch, strings.Join(again.Missing, ", "), linked.Shown(topo.File))
	}
	return again, nil
}

// describe counts the agents, partition agents, and sidecars of a topology.
func describe(topo *topology.Topology) string {
	parts := []string{plural(len(topo.Agents), "agent")}
	if n := len(topo.PartitionAgents()); n > 0 {
		parts = append(parts, plural(n, "partition"))
	}
	if n := len(topo.Services) - len(topo.Agents); n > 0 {
		parts = append(parts, plural(n, "sidecar"))
	}
	return strings.Join(parts, " · ")
}

// prepared says what partitions.New prepared.
func prepared(topo *topology.Topology) string {
	var repos []string
	for _, name := range slices.Sorted(maps.Keys(topo.Repos)) {
		repos = append(repos, fmt.Sprintf("%s on %s", name, topo.Repos[name].Branch))
	}
	if len(repos) == 0 {
		return "Prepared the vault"
	}
	return "Prepared " + strings.Join(repos, ", ") + " and the vault"
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// launch builds the project's images, checks each agent's image, writes the
// override file, and starts the project. The returned function stops the
// project; it is safe to call more than once, and it is not nil once
// containers may exist.
func launch(ctx context.Context, con *console.Console, p *launcher.Project, srv *server.Server, in launcher.OverrideInput, runDir string) (func(), error) {
	topo := p.Topo
	fail := func(step *console.Step, err error) error {
		log.Print(err)
		step.Fail(err)
		return errShown
	}
	step := con.Step("Checking the " + agentimage.Name + " image")
	state, err := agentimage.Ensure(ctx, log.Writer())
	if err != nil {
		return nil, fail(step, err)
	}
	switch state {
	case agentimage.Missing:
		step.Done("Built the " + agentimage.Name + " image")
	case agentimage.Stale:
		step.Done("Built the " + agentimage.Name + " image again, from a newer Dockerfile")
	default:
		step.Done("Checked the " + agentimage.Name + " image")
	}
	if slices.ContainsFunc(slices.Collect(maps.Values(topo.Services)), func(s topology.Service) bool { return s.Build }) {
		step := con.Step("Building the images")
		if err := p.Build(ctx); err != nil {
			return nil, fail(step, err)
		}
		step.Done("Built the images")
	}
	step = con.Step("Checking the agent images")
	in.Binaries, in.Users = map[string]string{}, map[string]string{}
	for _, n := range topo.AgentNames() {
		img, err := p.InspectAgent(ctx, n)
		if err != nil {
			return nil, fail(step, err)
		}
		if in.Binaries[n], err = launcher.LinuxBinary(img.Arch); err != nil {
			return nil, fail(step, err)
		}
		in.Users[n] = img.User
	}
	step.Done(fmt.Sprintf("Checked %s", plural(len(topo.Agents), "agent image")))
	if err := p.WriteOverride(filepath.Join(runDir, "compose.override.yaml"), in); err != nil {
		return nil, err
	}
	for _, n := range topo.AgentNames() {
		p.Env = append(p.Env, launcher.TokenVar(topo.Agents[n].ID)+"="+srv.IssueToken(n))
	}
	err = p.Watch(ctx, func(service string, code int) {
		log.Printf("[%s] container exited with code %d", service, code)
		if agent, ok := topo.AgentOfService(service); ok {
			s := api.Status{State: api.StateExited}
			if code != 0 && ctx.Err() == nil {
				s.Detail = fmt.Sprintf("exit code %d", code)
			}
			srv.SetStatus(agent, s)
		}
	})
	if err != nil {
		return nil, err
	}
	step = con.Step("Starting the containers")
	stop := sync.OnceFunc(p.Down)
	if err := p.Up(ctx); err != nil {
		return stop, fail(step, fmt.Errorf("%w; see the log", err))
	}
	p.Logs()
	step.Done(fmt.Sprintf("Started %s", plural(len(topo.Services), "container")))
	return stop, nil
}

func runDriver(ctx context.Context) error {
	log.SetFlags(0)
	c, err := api.ClientFromEnv()
	if err != nil {
		return err
	}
	err = driver.Run(ctx, c)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runMCP(ctx context.Context) error {
	c, err := api.ClientFromEnv()
	if err != nil {
		return err
	}
	self, err := c.Self(ctx)
	if err != nil {
		return err
	}
	return mcpserver.New(c, self).Run(ctx, &mcp.StdioTransport{})
}

func buildImage(ctx context.Context, args []string) error {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "Usage: hikari image")
		os.Exit(2)
	}
	if err := agentimage.Build(ctx, os.Stdout); err != nil {
		return err
	}
	fmt.Println("Built the " + agentimage.Name + " image.")
	return nil
}

// userPath reports whether path, relative to the project repository, holds
// your knowledge: a wiki document, or a document of a module (rule MO4 of
// the task pipeline spec).
func userPath(topo *topology.Topology, path string) bool {
	vaultRel := topo.VaultInProject()
	if partitions.IsUserPath(vaultRel, path) {
		return true
	}
	rel, ok := strings.CutPrefix(path, vaultRel+"/")
	return ok && modules.UserPath(topo, rel)
}
