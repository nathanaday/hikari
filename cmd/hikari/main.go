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
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/launcher"
	"github.com/nathanaday/hikari/internal/mcpserver"
	"github.com/nathanaday/hikari/internal/node"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/topology"
)

const usage = `hikari runs Claude Code agents in containers that talk to each other only through shared hubs.

Usage:
  hikari up [flags] <topology.yaml>   start the server, the web UI, and one container per agent
  hikari node                         run the agent driver (inside a container)
  hikari mcp                          run the MCP server on stdio (started by Claude Code)
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
	image := fs.String("image", "hikari-node", "agent container image")
	noLaunch := fs.Bool("no-launch", false, "start only the server and print agent credentials, for running agents by hand")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hikari up [flags] <topology.yaml>")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	topo, err := topology.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid -listen: %w", err)
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

	srv := server.New(topo, journal)
	uiToken := server.NewToken()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: srv.Handler(uiToken)}
	go httpSrv.Serve(ln)
	defer httpSrv.Close()

	l := &launcher.Launcher{Image: *image, ServerURL: "http://host.docker.internal:" + port, RunID: runID}
	if *noLaunch {
		for _, n := range topo.AgentNames() {
			fmt.Printf("%s: HIKARI_SERVER_URL=http://%s HIKARI_TOKEN=%s\n", n, *listen, srv.IssueToken(n))
		}
	} else {
		if err := l.CheckImage(ctx); err != nil {
			return err
		}
		defer l.Cleanup()
		for _, n := range topo.AgentNames() {
			err := l.Start(context.Background(), n, srv.IssueToken(n), func(err error) {
				s := api.Status{State: api.StateExited}
				if err != nil && ctx.Err() == nil {
					s.Detail = err.Error()
				}
				srv.SetStatus(n, s)
				log.Printf("[%s] container exited", n)
			})
			if err != nil {
				return err
			}
		}
	}

	log.Printf("journal: %s", journalPath)
	log.Printf("web UI:  http://%s/?token=%s", ln.Addr(), uiToken)
	<-ctx.Done()
	log.Print("shutting down")
	return nil
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
	return mcpserver.New(c).Run(ctx, &mcp.StdioTransport{})
}
