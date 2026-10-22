package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/launcher"
	"github.com/nathanaday/hikari/internal/mcpserver"
	"github.com/nathanaday/hikari/internal/node"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/systems"
	"github.com/nathanaday/hikari/internal/topology"
)

const usage = `hikari runs Claude Code agents in containers that talk to each other only through shared hubs.

Usage:
  hikari up [flags] <compose.yaml>    start the server, the web UI, and the compose project

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
	case "node":
		err = runNode(ctx)
	case "mcp":
		err = runMCP(ctx)
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

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

	topo, err := topology.Load(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid -listen: %w", err)
	}
	project := &launcher.Project{Topo: topo}
	if !*noLaunch {
		running, err := project.Running(ctx)
		if err != nil {
			return err
		}
		if running {
			return fmt.Errorf("containers of project %q are running; stop them with `docker compose -p %s down`", topo.Name, topo.Name)
		}
		for _, n := range topo.AgentNames() {
			if !topo.Services[n].Credentials() {
				log.Printf("warning: agent %q has neither ANTHROPIC_API_KEY nor CLAUDE_CODE_OAUTH_TOKEN set", n)
			}
		}
	}

	runID := time.Now().Format("20060102-150405")
	if err := os.MkdirAll(".hikari", 0o755); err != nil {
		return err
	}
	journalPath := filepath.Join(".hikari", runID+".jsonl")
	journal, err := os.Create(journalPath)
	if err != nil {
		return err
	}
	defer journal.Close()

	// Listen first, so that a busy port stops hikari before it touches any
	// repo.
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	sys, err := systems.New(topo)
	if err != nil {
		return err
	}
	for _, w := range sys.Warnings() {
		log.Printf("warning: %s", w)
	}
	sys.Watch(ctx)
	srv := server.New(topo, journal, sys)
	uiToken := server.NewToken()
	httpSrv := &http.Server{Handler: srv.Handler(uiToken)}
	go httpSrv.Serve(ln)
	defer httpSrv.Close()

	// Containers stop before the final commits, so that no agent writes
	// during them.
	stopContainers := func() {}
	if *noLaunch {
		for _, n := range topo.AgentNames() {
			fmt.Printf("%s: HIKARI_SERVER_URL=http://%s HIKARI_TOKEN=%s\n", n, *listen, srv.IssueToken(n))
		}
	} else {
		emptyDir, err := os.MkdirTemp("", "hikari-empty-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(emptyDir)
		stop, err := launch(ctx, project, srv, launcher.OverrideInput{
			RunID:     runID,
			ServerURL: "http://host.docker.internal:" + port,
			EmptyDir:  emptyDir,
		})
		if err != nil {
			return err
		}
		defer stop()
		stopContainers = stop
	}

	log.Printf("journal: %s", journalPath)
	log.Printf("web UI:  http://%s/?token=%s", ln.Addr(), uiToken)
	<-ctx.Done()
	log.Print("shutting down")
	stopContainers()
	for _, err := range sys.CommitAll() {
		log.Printf("final commit: %v", err)
	}
	for _, line := range sys.Summary() {
		log.Print(line)
	}
	return nil
}

// launch builds the project's images, checks each agent's image, writes the
// override file, and starts the project. The returned function stops it;
// it is safe to call more than once.
func launch(ctx context.Context, p *launcher.Project, srv *server.Server, in launcher.OverrideInput) (func(), error) {
	topo := p.Topo
	if err := p.Build(ctx); err != nil {
		return nil, err
	}
	in.Binaries, in.Users = map[string]string{}, map[string]string{}
	for _, n := range topo.AgentNames() {
		img, err := p.InspectAgent(ctx, n)
		if err != nil {
			return nil, err
		}
		if in.Binaries[n], err = launcher.LinuxBinary(img.Arch); err != nil {
			return nil, err
		}
		in.Users[n] = img.User
	}
	if err := p.WriteOverride(filepath.Join(".hikari", in.RunID, "compose.override.yaml"), in); err != nil {
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
	stop := sync.OnceFunc(p.Down)
	if err := p.Up(ctx); err != nil {
		stop()
		return nil, err
	}
	p.Logs()
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
