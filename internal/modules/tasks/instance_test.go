package tasks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/nathanaday/hikari/internal/topology"
)

type commit struct {
	author, message string
	paths           []string
}

// fakeHost records what the module asks of hikari.
type fakeHost struct {
	mu      sync.Mutex
	topo    *topology.Topology
	vault   string
	health  map[string]string
	commits []commit
	notes   []Note
}

func (h *fakeHost) Topology() *topology.Topology { return h.topo }
func (h *fakeHost) VaultDir() string             { return h.vault }
func (h *fakeHost) Health(p, id string) (string, bool) {
	v, ok := h.health[p+"/"+id]
	return v, ok
}
func (h *fakeHost) Commit(author, message string, paths ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commits = append(h.commits, commit{author, message, paths})
}
func (h *fakeHost) Notify(to, body string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notes = append(h.notes, Note{To: to, Body: body})
}
func (h *fakeHost) Changed() {}

func (h *fakeHost) authors() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, c := range h.commits {
		out = append(out, c.author)
	}
	return out
}

func shopHost(t *testing.T) *fakeHost {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "shop", ".git"), 0o755)
	for _, d := range []string{"orders", "inventory"} {
		os.MkdirAll(filepath.Join(dir, "shop", d), 0o755)
	}
	topo, err := topology.Parse([]byte(`name: shop
x-hikari:
  repos: {shop: {path: shop, branch: hikari}}
  orchestration: lead
  modules: [tasks]
services:
  lead: {x-hikari: {}}
  orders: {x-hikari: {partition: {folders: [{repo: shop, path: orders}]}}}
  inventory: {x-hikari: {partition: {folders: [{repo: shop, path: inventory}]}}}
`), dir)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeHost{topo: topo, vault: topo.Vault, health: map[string]string{"orders/create-order": "passing"}}
}

// TestInstance checks G3, TK7, TK19, and TK24 through the host half.
func TestInstance(t *testing.T) {
	h := shopHost(t)
	mi, err := Module{}.Open(h)
	if err != nil {
		t.Fatal(err)
	}
	i := mi.(*instance)
	i.do("plant", func(c *Change) (any, error) {
		_, paths, err := i.store.PlantStub("Cancel", "Cancel orders.", "inventory")
		c.wrote("inventory", paths...)
		return nil, err
	})
	i.do("scope", func(c *Change) (any, error) {
		_, paths, err := i.store.Scope(ScopeInput{ID: "cancel", Partitions: []string{"orders"}, Goal: "g", Parts: map[string]string{"orders": "x"}, DoneWhen: "d"})
		c.wrote("lead", paths...)
		return nil, err
	})
	i.do("queue", func(c *Change) (any, error) {
		paths, err := i.store.ChangeQueue(QueueOp{Op: "create", Name: "Q"})
		c.wrote(User, paths...)
		p2, err2 := i.store.ChangeQueue(QueueOp{Op: "add", Queue: "q", Scope: "cancel"})
		c.wrote(User, p2...)
		return nil, errorsJoin(err, err2)
	})
	i.do("start", func(c *Change) (any, error) { return nil, i.store.SetQueueState("q", Running, c) })
	if got := h.authors(); !slices.Equal(got, []string{"inventory", "lead", User, User, Hikari}) {
		t.Errorf("G3 each write is committed as its writer: %v", got)
	}
	if len(h.notes) != 1 || h.notes[0].To != "lead" || !strings.Contains(h.notes[0].Body, "started [[cancel]]") {
		t.Errorf("TK13 hikari tells lead: %+v", h.notes)
	}
	if n := i.Notices("orders"); len(n) != 1 || !strings.Contains(n[0], "your part of [[cancel]] is queued, and the scope is started") {
		t.Errorf("TK24 orders' notice: %v", n)
	}
	if n := i.Notices("lead"); len(n) != 1 || !strings.Contains(n[0], "[[cancel]] is started (orders queued)") {
		t.Errorf("TK24 lead's notice: %v", n)
	}
	if n := i.Notices("inventory"); len(n) != 0 {
		t.Errorf("TK24 inventory has no part: %v", n)
	}

	// TK7: your edit in Obsidian is committed as yours, and a new stub of
	// yours is completed by hikari.
	os.WriteFile(filepath.Join(h.vault, Dir, "gift-wrap.md"), []byte("---\nstage: stub\n---\nWrap gifts.\n"), 0o644)
	before := len(h.authors())
	i.reload()
	got := h.authors()[before:]
	if !slices.Equal(got, []string{User, Hikari}) || !slices.Contains(h.commits[before].paths, "tasks/gift-wrap.md") {
		t.Errorf("TK7 your edit, then hikari's fill: %v %+v", got, h.commits[before:])
	}
	v := i.View().(View)
	if len(v.Stubs) != 1 || v.Stubs[0].ID != "gift-wrap" || len(v.Queues) != 1 || v.Queues[0].Scopes[0].Status != Started {
		t.Errorf("the view: %+v", v)
	}
}

func errorsJoin(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func TestStubName(t *testing.T) {
	for in, want := range map[string][2]string{
		"Coupons at checkout":                        {"Coupons at checkout", ""},
		"Coupons at checkout.\nThe code is printed.": {"Coupons at checkout", "Coupons at checkout.\nThe code is printed."},
		strings.Repeat("word ", 20):                  {strings.TrimSpace(strings.Repeat("word ", 12)) + "…", strings.TrimSpace(strings.Repeat("word ", 20))},
	} {
		name, body := stubName(strings.TrimSpace(in))
		if name != want[0] || body != want[1] {
			t.Errorf("stubName(%q) = %q, %q", in, name, body)
		}
	}
}
