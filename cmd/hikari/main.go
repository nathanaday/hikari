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
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/gitrepo"
	"github.com/nathanaday/hikari/internal/launcher"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/mcpserver"
	"github.com/nathanaday/hikari/internal/node"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/systems"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/version"
)

const usage = `hikari runs Claude Code agents in containers that talk to each other only through shared hubs.

Usage:
  hikari up [flags] <compose.yaml>    start the server, the web UI, and the compose project
  hikari init [flags] <dir>           make a new project repository
  hikari sync [flags] <compose.yaml>  clone the linked repos, and make your branches from the remote
  hikari check <compose.yaml>         run the checks of hikari up, and start nothing
  hikari push <compose.yaml>          push your branches of the linked repos and the project
  hikari version                      print the version

Internal commands (hikari starts these; you do not run them):
  hikari node   the agent driver; each container runs it
  hikari mcp    the agent's MCP tools; Claude Code starts it on each turn
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
	case "node":
		err = runNode(ctx)
	case "mcp":
		err = runMCP(ctx)
	case "version", "-v", "-version", "--version":
		fmt.Println("hikari " + version.Version)
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
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
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hikari up [flags] <compose.yaml>")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	_, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid -listen: %w", err)
	}

	// Everything for the record goes to the run's log; the terminal shows
	// what a person needs.
	runID := time.Now().Format("20060102-150405")
	runDir := filepath.Join(".hikari", "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(runDir, "hikari.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	con := console.New(os.Stdout)
	printBanner(con)
	fail := func(step *console.Step, err error) error {
		log.Print(err)
		step.Fail(err)
		return errShown
	}

	step := con.Step("Reading " + fs.Arg(0))
	topo, err := topology.Load(ctx, fs.Arg(0))
	if err != nil {
		return fail(step, err)
	}
	project := &launcher.Project{Topo: topo}
	if !*noLaunch {
		running, err := project.Running(ctx)
		if err != nil {
			return fail(step, err)
		}
		if running {
			return fail(step, fmt.Errorf("containers of project %q are running; stop them with `docker compose -p %s down`", topo.Name, topo.Name))
		}
	}
	findings := linked.Check(topo, !*noLaunch)
	if slices.ContainsFunc(findings, func(f linked.Finding) bool { return !f.Warning }) {
		step.Fail(errors.New("the project cannot start"))
		for _, w := range topo.Warnings {
			con.Warn(w)
		}
		showFindings(con, findings)
		return errShown
	}
	step.Done(fmt.Sprintf("Read %s %s", con.Bold(topo.Name), con.Dim("· "+describe(topo))))
	for _, w := range topo.Warnings {
		con.Warn(w)
	}
	showFindings(con, findings)

	// Listen first, so that a busy port stops hikari before it touches any
	// repo.
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Print(err)
		con.Error(fmt.Sprintf("cannot listen on %s; pass another address with -listen (%v)", *listen, err))
		return errShown
	}
	step = con.Step("Preparing the repos and the store")
	if topo, err = prepareProject(ctx, topo); err != nil {
		return fail(step, err)
	}
	project.Topo = topo
	sys, err := systems.New(topo)
	if err != nil {
		return fail(step, err)
	}
	step.Done(prepared(topo))
	for _, w := range sys.Warnings() {
		log.Printf("warning: %s", w)
		con.Warn(w)
	}
	sys.SetRun(runID)
	sys.Watch(ctx)
	journal := &lazyFile{path: filepath.Join(runDir, "messages.jsonl")}
	defer journal.Close()
	srv := server.New(topo, journal, sys)
	uiToken := server.NewToken()
	httpSrv := &http.Server{Handler: srv.Handler(uiToken)}
	go httpSrv.Serve(ln)
	defer httpSrv.Close()
	url := fmt.Sprintf("http://%s/?token=%s", ln.Addr(), uiToken)
	log.Printf("web UI: %s", url)
	dash := newDashboard(con, srv, topo.AgentNames(), url, logPath)

	// Containers stop before the final commits, so that no agent writes
	// during them.
	stopContainers := func() {}
	if *noLaunch {
		for _, n := range topo.AgentNames() {
			con.Println(fmt.Sprintf("  %s  HIKARI_SERVER_URL=http://%s HIKARI_TOKEN=%s", con.Bold(n), *listen, srv.IssueToken(n)))
		}
	} else {
		emptyDir, err := os.MkdirTemp("", "hikari-empty-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(emptyDir)
		stop, err := launch(ctx, con, project, srv, launcher.OverrideInput{
			RunID:     runID,
			ServerURL: "http://host.docker.internal:" + port,
			EmptyDir:  emptyDir,
		}, runDir)
		if stop != nil {
			defer stop()
			stopContainers = stop
		}
		if err != nil {
			if !errors.Is(err, errShown) {
				log.Print(err)
				con.Error(err.Error())
			}
			return errShown
		}
		step = con.Step("Waiting for the agents to connect")
		if dash.waitConnected(ctx, time.Minute) {
			step.Done(fmt.Sprintf("Connected %d agents", len(topo.Agents)))
		} else if ctx.Err() == nil {
			step.Fail(fmt.Errorf("not every agent connected within a minute; see %s", logPath))
		}
	}

	con.SetFooter(dash.footer)
	<-ctx.Done()
	log.Print("shutting down")
	con.ClearFooter()
	con.Println("")
	sys.StopEnvironments(context.Background())
	if !*noLaunch {
		step = con.Step("Stopping the containers")
		stopContainers()
		step.Done("Stopped the containers")
	}
	if len(topo.Owners()) > 0 {
		step = con.Step("Committing what the owners left")
		for _, err := range sys.CommitAll() {
			log.Printf("final commit: %v", err)
			con.Error(fmt.Sprintf("final commit: %v", err))
		}
		summary := sys.Summary()
		for _, line := range summary {
			log.Print(line)
		}
		if len(summary) == 0 {
			summary = []string{"No commits"}
		}
		step.Done(summary[0])
		for _, line := range summary[1:] {
			con.Println("    " + line)
		}
	}
	con.Println("  " + con.Dim("The run's log and messages are in "+runDir))
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
		if changed, err := repo.Changed(); err == nil && len(changed) > 0 && !slices.ContainsFunc(changed, func(path string) bool { return !systems.IsWikiPath(topo.StoreInProject(), path) }) {
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
		return nil, fmt.Errorf("the compose file on the project branch %s: %w", p.Branch, err)
	}
	if again.Project == nil || again.Project.Branch != p.Branch {
		return nil, fmt.Errorf("the compose file on the project branch %s names another project branch, or none; make the branches agree", p.Branch)
	}
	if len(again.Missing) > 0 {
		return nil, fmt.Errorf("the compose file on the project branch %s links repos that are not cloned (%s); run hikari sync %s", p.Branch, strings.Join(again.Missing, ", "), linked.Shown(topo.File))
	}
	return again, nil
}

// describe counts the agents, owners, and sidecars of a topology.
func describe(topo *topology.Topology) string {
	parts := []string{plural(len(topo.Agents), "agent")}
	if n := len(topo.Owners()); n > 0 {
		parts = append(parts, plural(n, "system"))
	}
	if n := len(topo.Services) - len(topo.Agents); n > 0 {
		parts = append(parts, plural(n, "sidecar"))
	}
	return strings.Join(parts, " · ")
}

// prepared says what systems.New prepared.
func prepared(topo *topology.Topology) string {
	var repos []string
	for _, name := range slices.Sorted(maps.Keys(topo.Repos)) {
		repos = append(repos, fmt.Sprintf("%s on %s", name, topo.Repos[name].Branch))
	}
	if len(repos) == 0 {
		return "Prepared the store"
	}
	return "Prepared " + strings.Join(repos, ", ") + " and the store"
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
	if slices.ContainsFunc(slices.Collect(maps.Values(topo.Services)), func(s topology.Service) bool { return s.Build }) {
		step := con.Step("Building the images")
		if err := p.Build(ctx); err != nil {
			return nil, fail(step, err)
		}
		step.Done("Built the images")
	}
	step := con.Step("Checking the agent images")
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
		p.Env = append(p.Env, launcher.TokenVar(n)+"="+srv.IssueToken(n))
	}
	err := p.Watch(ctx, func(service string, code int) {
		log.Printf("[%s] container exited with code %d", service, code)
		if _, ok := topo.Agents[service]; ok {
			s := api.Status{State: api.StateExited}
			if code != 0 && ctx.Err() == nil {
				s.Detail = fmt.Sprintf("exit code %d", code)
			}
			srv.SetStatus(service, s)
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

func runNode(ctx context.Context) error {
	log.SetFlags(0)
	c, err := api.ClientFromEnv()
	if err != nil {
		return err
	}
	err = node.Run(ctx, c)
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
