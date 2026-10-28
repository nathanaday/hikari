package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/hostfile"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/scaffold"
)

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
// server of the running hikari up for file (rule CP12 of the plugin spec).
func sendMessage(ctx context.Context, file, to, body string) (api.Message, error) {
	var m api.Message
	err := hostfile.Call(ctx, file, linked.Shown(file), http.MethodPost, "/api/ui/send", api.SendRequest{To: to, Body: body}, &m)
	return m, err
}
