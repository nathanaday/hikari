package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/editor"
	"github.com/nathanaday/hikari/internal/launcher"
	"github.com/nathanaday/hikari/internal/linked"
	"github.com/nathanaday/hikari/internal/partitions"
	"github.com/nathanaday/hikari/internal/server"
	"github.com/nathanaday/hikari/internal/topology"
)

// runner starts and stops the agents of one topology file, for the life of
// hikari up (section 8 of the topology API spec). The host serves the UI
// in every mode.
type runner struct {
	ctx      context.Context
	con      *console.Console
	file     string
	listen   string
	port     string
	noLaunch bool
	url      string
	host     *server.Host

	mu  sync.Mutex
	cur *run
}

// run is one start of the agents: its own folder in .hikari/runs (rule
// UP7).
type run struct {
	dir     string
	logFile *os.File
	journal *lazyFile
	sys     *partitions.Manager
	topo    *topology.Topology
	stop    func()
}

// editLog is where the log goes while no run is on.
func editLog() io.Writer {
	if err := os.MkdirAll(".hikari", 0o755); err != nil {
		return io.Discard
	}
	f, err := os.OpenFile(filepath.Join(".hikari", "up.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return io.Discard
	}
	return f
}

// start runs the start sequence. When it fails, hikari up waits in the
// editing state with the findings (rules UP2 and UP4).
func (r *runner) start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur != nil {
		return
	}
	cur, findings := r.startRun()
	if cur == nil {
		log.SetOutput(editLog())
		if r.ctx.Err() == nil {
			r.con.Println("  " + r.con.Yellow("…") + " Editing " + linked.Shown(r.file) + ": open the web UI to fix it, then start the agents there")
		}
		r.host.Editing(findings)
		return
	}
	r.cur = cur
}

func fail(step *console.Step, err error) []topology.Finding {
	log.Print(err)
	step.Fail(err)
	return []topology.Finding{{Severity: topology.SeverityError, Stage: topology.StageStart, Message: err.Error()}}
}

func (r *runner) startRun() (*run, []topology.Finding) {
	ctx, con := r.ctx, r.con
	if _, err := os.Stat(r.file); errors.Is(err, os.ErrNotExist) {
		// The UI's Create the project says this already.
		con.Println("  " + con.Yellow("!") + " " + linked.Shown(r.file) + " does not exist; create it in the web UI")
		return nil, nil
	}
	// Everything for the record goes to the run's log; the terminal shows
	// what a person needs.
	runID := time.Now().Format("20060102-150405")
	// A start in the same second as the last one gets a suffix.
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(".hikari", "runs", runID)); errors.Is(err, os.ErrNotExist) {
			break
		}
		runID = fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), n)
	}
	runDir := filepath.Join(".hikari", "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fail(con.Step("Making "+runDir), err)
	}
	logPath := filepath.Join(runDir, "hikari.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fail(con.Step("Making "+logPath), err)
	}
	log.SetOutput(logFile)
	cur := &run{dir: runDir, logFile: logFile}
	ok := false
	defer func() {
		if !ok {
			cur.close()
		}
	}()

	step := con.Step("Reading " + linked.Shown(r.file))
	topo, err := topology.Load(ctx, r.file)
	if err != nil {
		log.Print(err)
		step.Fail(errors.New("the topology file has errors"))
		src, _ := os.ReadFile(r.file)
		findings := topology.Findings(err, src)
		for _, f := range findings {
			con.Error(f.Message)
		}
		return nil, findings
	}
	project := &launcher.Project{Topo: topo}
	if !r.noLaunch {
		running, err := project.Running(ctx)
		if err != nil {
			return nil, fail(step, err)
		}
		if running {
			return nil, fail(step, fmt.Errorf("containers of project %q are running; stop them with `docker compose -p %s down`", topo.ID, topo.ID))
		}
	}
	checks := linked.Check(topo, !r.noLaunch)
	if slices.ContainsFunc(checks, func(f linked.Finding) bool { return !f.Warning }) {
		step.Fail(errors.New("the project cannot start"))
		for _, w := range topo.Warnings {
			con.Warn(w)
		}
		showFindings(con, checks)
		var findings []topology.Finding
		for _, f := range checks {
			sev := topology.SeverityError
			if f.Warning {
				sev = topology.SeverityWarning
			}
			findings = append(findings, topology.Finding{Severity: sev, Stage: topology.StageStart, Message: f.Text})
		}
		return nil, findings
	}
	step.Done(fmt.Sprintf("Read %s %s", con.Bold(topo.Name), con.Dim("· "+describe(topo))))
	for _, w := range topo.Warnings {
		con.Warn(w)
	}
	showFindings(con, checks)

	step = con.Step("Preparing the repos and the vault")
	if topo, err = prepareProject(ctx, topo); err != nil {
		return nil, fail(step, err)
	}
	project.Topo = topo
	sys, err := partitions.New(topo)
	if err != nil {
		return nil, fail(step, err)
	}
	step.Done(prepared(topo))
	for _, w := range sys.Warnings() {
		log.Printf("warning: %s", w)
		con.Warn(w)
	}
	runCtx, cancelWatch := context.WithCancel(ctx)
	sys.Watch(runCtx)
	cur.journal = &lazyFile{path: filepath.Join(runDir, "messages.jsonl")}
	srv := server.New(topo, cur.journal, sys)
	srv.WatchModules(runCtx)
	r.host.Attach(srv)
	cur.sys, cur.topo = sys, topo
	dash := newDashboard(con, srv, topo.AgentNames(), r.url, logPath)

	// Containers stop before the final commits, so that no agent writes
	// during them.
	stopContainers := func() {}
	if r.noLaunch {
		for _, n := range topo.AgentNames() {
			con.Println(fmt.Sprintf("  %s  HIKARI_SERVER_URL=http://%s HIKARI_TOKEN=%s", con.Bold(n), r.listen, srv.IssueToken(n)))
		}
	} else {
		emptyDir, err := os.MkdirTemp("", "hikari-empty-")
		if err != nil {
			cancelWatch()
			return nil, fail(step, err)
		}
		stop, err := launch(ctx, con, project, srv, launcher.OverrideInput{
			RunID:     runID,
			ServerURL: "http://host.docker.internal:" + r.port,
			EmptyDir:  emptyDir,
		}, runDir)
		if err != nil {
			if stop != nil {
				stop()
			}
			os.RemoveAll(emptyDir)
			cancelWatch()
			if !errors.Is(err, errShown) {
				log.Print(err)
				con.Error(err.Error())
			}
			return nil, []topology.Finding{{Severity: topology.SeverityError, Stage: topology.StageStart, Message: "the containers did not start; see " + logPath}}
		}
		stopContainers = func() { stop(); os.RemoveAll(emptyDir) }
		step = con.Step("Waiting for the agents to connect")
		if dash.waitConnected(ctx, time.Minute) {
			step.Done(fmt.Sprintf("Connected %d agents", len(topo.Agents)))
		} else if ctx.Err() == nil {
			step.Fail(fmt.Errorf("not every agent connected within a minute; see %s", logPath))
		}
	}
	cur.stop = func() {
		if !r.noLaunch {
			step := con.Step("Stopping the containers")
			stopContainers()
			step.Done("Stopped the containers")
		}
		cancelWatch()
	}

	var commit editor.Committer
	if topo.Project != nil {
		commit = sys.CommitAsUser
	}
	r.host.Running(srv, commit)
	con.SetFooter(dash.footer)
	ok = true
	return cur, nil
}

func (cur *run) close() {
	if cur.journal != nil {
		cur.journal.Close()
	}
	cur.logFile.Close()
}

// stop stops the agents and commits what the partition agents left (rule UP6). It
// returns to the editing state unless hikari exits.
func (r *runner) stop(exiting bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.cur
	if cur == nil {
		return
	}
	r.cur = nil
	con := r.con
	log.Print("stopping the run")
	con.ClearFooter()
	con.Println("")
	cur.stop()
	if len(cur.topo.PartitionAgents()) > 0 {
		step := con.Step("Committing what the partition agents left")
		for _, err := range cur.sys.CommitAll() {
			log.Printf("final commit: %v", err)
			con.Error(fmt.Sprintf("final commit: %v", err))
		}
		summary := cur.sys.Summary()
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
	con.Println("  " + con.Dim("The run's log and messages are in "+cur.dir))
	cur.close()
	log.SetOutput(editLog())
	if !exiting {
		con.Println("  " + con.Yellow("…") + " Editing " + linked.Shown(r.file) + ": start the agents again in the web UI, or press Ctrl-C to quit")
		r.host.Editing(nil)
	}
}
