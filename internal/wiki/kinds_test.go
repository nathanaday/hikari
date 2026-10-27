package wiki

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var systemKinds = []string{Feature, Principle, Rule}
var publicKinds = []string{Story, Rule, Principle}

func TestKinds(t *testing.T) {
	c := Check{Kinds: systemKinds}
	w, _, err := Apply(Wiki{}, []Write{
		{Op: "create", ID: "reserve-stock", Name: "Reserve stock", Description: "Hold stock.", Body: body("Holds stock; keeps [[never-negative]].")},
		{Op: "create", ID: "never-negative", Kind: Rule, Name: "Never negative", Description: "Stock never goes below zero.", Body: body("Why.")},
	}, c, now)
	if err != nil {
		t.Fatal(err)
	}
	if w["reserve-stock"].Kind != Feature || w["never-negative"].KindOf() != Rule {
		t.Errorf("a create without a kind is a feature: %q, %q", w["reserve-stock"].Kind, w["never-negative"].Kind)
	}
	if got := w.Features(); !slices.Equal(got, []string{"reserve-stock"}) {
		t.Errorf("Features() = %v", got)
	}
	if n := w.Neighbors(); len(n) != 0 {
		t.Errorf("N7: a rule is no neighbor in the map: %v", n)
	}
	if ws := w.Warnings(c); len(ws) != 0 {
		t.Errorf("N7: a feature that links a rule has a neighbor: %v", ws)
	}
	for name, tc := range map[string]struct {
		w    Write
		c    Check
		want string
	}{
		"story in a system":          {Write{Op: "create", ID: "s", Kind: Story, Name: "S", Description: "D.", Body: body("p")}, c, "holds feature, principle, rule"},
		"no kind in the public wiki": {Write{Op: "create", ID: "s", Name: "S", Description: "D.", Body: body("p")}, Check{Kinds: publicKinds}, "needs a kind"},
		"a plan":                     {Write{Op: "create", ID: "p", Kind: Plan, Name: "P", Description: "D.", Body: body("p")}, c, "T3"},
		"interfaces":                 {Write{Op: "create", ID: "p", Kind: Principle, Name: "P", Description: "D.", Interfaces: []string{"x"}, Body: body("p")}, c, "W3"},
	} {
		if _, _, err := Apply(w, []Write{tc.w}, tc.c, now); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	// T3: a user document is not the agent's.
	w["Roadmap"] = &Document{ID: "Roadmap", Kind: Plan, Body: "Next: [[reserve-stock]]."}
	for _, wr := range []Write{{Op: "modify", ID: "Roadmap", Body: body("x")}, {Op: "remove", ID: "Roadmap"}, {Op: "rename", ID: "Roadmap", To: "x"}} {
		if _, _, err := Apply(w, []Write{wr}, c, now); err == nil || !strings.Contains(err.Error(), "T3") {
			t.Errorf("%s of a plan: %v", wr.Op, err)
		}
	}
	// An agent document cannot link a user document.
	if _, _, err := Apply(w, []Write{{Op: "modify", ID: "reserve-stock", Body: body("See [[Roadmap]].")}}, c, now); err == nil || !strings.Contains(err.Error(), "K2") {
		t.Errorf("a link to a plan: %v", err)
	}
}

func TestLinksAcrossWikis(t *testing.T) {
	pub := Check{Kinds: publicKinds, Linkable: func(id string) bool { return id == "reserve-stock" }, Taken: map[string]bool{"reserve-stock": true}}
	w, _, err := Apply(Wiki{}, []Write{{Op: "create", ID: "order-items", Kind: Story, Name: "Order items", Description: "A customer orders.", Body: body("Through [[systems/inventory/wiki/reserve-stock]] and [[reserve-stock.md|stock]].")}}, pub, now)
	if err != nil {
		t.Fatalf("N1, N2: %v", err)
	}
	if got := w["order-items"].Neighbors(); !slices.Equal(got, []string{"reserve-stock"}) {
		t.Errorf("N1: links name the last path segment: %v", got)
	}
	if _, _, err := Apply(w, []Write{{Op: "create", ID: "reserve-stock", Kind: Rule, Name: "R", Description: "D.", Body: body("p")}}, pub, now); err == nil || !strings.Contains(err.Error(), "W1") {
		t.Errorf("W1: an id another wiki uses: %v", err)
	}
	// N4: a hand edit that broke a link warns, and does not block a change
	// of another document.
	w["order-items"].Body = *body("Through [[gone]].")
	if ws := strings.Join(w.Warnings(pub), "\n"); !strings.Contains(ws, `links "gone"`) {
		t.Errorf("N4 warnings: %s", ws)
	}
	if _, _, err := Apply(w, []Write{{Op: "create", ID: "stock-rule", Kind: Rule, Name: "R", Description: "D.", Body: body("p")}}, pub, now); err != nil {
		t.Errorf("N4: %v", err)
	}
}

func TestUserDocuments(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Ideas for Q1.md"), []byte("Try [[reserve-stock]] and [[nowhere]].\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "odd.md"), []byte("---\nkind: wish\ntags: [x]\n---\nA wish.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "plan.md"), []byte("---\nkind: plan\nstatus: doing\nid: something-else\nrelated: [\"[[a#b]]\"]\n---\nPlan.\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "attachments"), 0o755)
	s, err := Open(dir, systemKinds)
	if err != nil {
		t.Fatal(err)
	}
	w := s.Wiki()
	if d := w["Ideas for Q1"]; d == nil || d.KindOf() != Note || d.ID != "Ideas for Q1" {
		t.Fatalf("D4: a file without a header is a note: %+v", d)
	}
	if d := w["plan"]; d == nil || d.KindOf() != Plan || d.Status != "doing" || d.ID != "plan" {
		t.Errorf("D2: a plan keeps its file name and status: %+v", d)
	}
	if !strings.Contains(s.Error(), "attachments/") {
		t.Errorf("D4: a folder is reported: %q", s.Error())
	}
	ws := strings.Join(w.Warnings(Check{Known: func(id string) bool { return id == "reserve-stock" }}), "\n")
	for _, want := range []string{`"wish", which hikari does not know`, `links "nowhere"`} {
		if !strings.Contains(ws, want) {
			t.Errorf("warnings lack %q:\n%s", want, ws)
		}
	}
	if strings.Contains(ws, `links "reserve-stock"`) {
		t.Errorf("N3: a user document may link any document of the store:\n%s", ws)
	}
	if p := UserNameProblem("a: b"); p == "" {
		t.Error("D2: a colon is refused")
	}
	if p := UserNameProblem(strings.Repeat("x", 201)); p == "" {
		t.Error("D2: a long name is refused")
	}

	// WriteDoc keeps a user document's text as it is, and RemoveUserDoc
	// removes it.
	text := "---\nkind: note\ntags: [b, a]\n---\n\nKeep  this  spacing.\n"
	if err := s.WriteDoc("Meeting", text, Check{}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "Meeting.md")); string(data) != text {
		t.Errorf("U6: the text changed:\n%s", data)
	}
	if err := s.WriteDoc("bad/name", "x", Check{}); err == nil {
		t.Error("D2: a slash is refused")
	}
	if err := s.RemoveUserDoc("Meeting"); err != nil || s.Wiki()["Meeting"] != nil {
		t.Errorf("remove: %v", err)
	}
	// An agent document written in the UI keeps the strict rules.
	agent := "---\nid: r\nkind: rule\nname: R\ndescription: D.\n---\n\n## Purpose\n\nSee [[gone]].\n"
	if err := s.WriteDoc("r", agent, Check{}); err == nil || !strings.Contains(err.Error(), "K2") {
		t.Errorf("U8: %v", err)
	}
	if err := s.WriteDoc("r", strings.Replace(agent, "See [[gone]].", "Why.", 1), Check{}); err != nil {
		t.Errorf("U8: a valid rule: %v", err)
	}
	if err := s.RemoveUserDoc("r"); err == nil {
		t.Error("U8: an agent document is not removed in the UI")
	}
}

func TestRelinkAcrossWikis(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, publicKinds)
	c := Check{Kinds: publicKinds, Linkable: func(string) bool { return true }}
	if _, err := s.Change([]Write{{Op: "create", ID: "order-items", Kind: Story, Name: "Order items", Description: "A customer orders.", Related: []string{"hold"}, Body: body("Through [[hold|holding]] and [[gone]].")}}, c, now); err != nil {
		t.Fatal(err)
	}
	note := "---\ntags: [z, a]\n---\nSee [[hold]] and [[gone|the old one]].\n"
	os.WriteFile(filepath.Join(dir, "Thoughts.md"), []byte(note), 0o644)
	s.Reload()
	written, err := s.Relink(map[string]string{"hold": "reserve-stock"}, map[string]string{"gone": ""})
	if err != nil || !slices.Equal(written, []string{"Thoughts", "order-items"}) {
		t.Fatalf("N5: written %v, %v", written, err)
	}
	d := s.Wiki()["order-items"]
	if !strings.Contains(d.Body, "[[reserve-stock|holding]]") || strings.Contains(d.Body, "[[gone]]") || !slices.Equal(d.Related, []string{"reserve-stock"}) {
		t.Errorf("N5: the story: %q %v", d.Body, d.Related)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Thoughts.md"))
	if string(data) != "---\ntags: [z, a]\n---\nSee [[reserve-stock]] and the old one.\n" {
		t.Errorf("N5: a user document gets only its link text replaced:\n%s", data)
	}

	// N5a: a file broken on disk gets its links replaced in its text, and a
	// change that names it is refused.
	os.WriteFile(filepath.Join(dir, "order-items.md"), []byte(s.Raw("order-items")+"\n---\nbroken: [\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "order-items.md"), []byte("---\nid: [\n---\n[[reserve-stock]]\n"), 0o644)
	s.Reload()
	if _, err := s.Relink(map[string]string{"reserve-stock": "hold-stock"}, nil); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "order-items.md")); !strings.Contains(string(data), "[[hold-stock]]") || !strings.HasPrefix(string(data), "---\nid: [\n") {
		t.Errorf("N5: a broken file gets its link text replaced:\n%s", data)
	}
	if _, err := s.Change([]Write{{Op: "modify", ID: "order-items", Body: body("x")}}, c, now); err == nil || !strings.Contains(err.Error(), "N5a") {
		t.Errorf("N5a: %v", err)
	}
}

func TestRender(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, publicKinds)
	c := Check{Kinds: publicKinds}
	if _, err := s.Change([]Write{
		{Op: "create", ID: "order-items", Kind: Story, Name: "Order items", Description: "A customer orders.", Body: body("p")},
	}, c, now); err != nil {
		t.Fatal(err)
	}
	raw := s.Raw("order-items")
	if !strings.Contains(raw, "\nkind: story\n") || strings.Contains(raw, "described_commit") {
		t.Errorf("D3: the story's header:\n%s", raw)
	}
	if got := s.Files(Story); len(got) != 1 {
		t.Errorf("L2: files of kinds: %v", got)
	}

	// The key flag of an older story is ignored.
	os.WriteFile(filepath.Join(dir, "order-items.md"), []byte(strings.Replace(raw, "\nkind: story\n", "\nkind: story\nkey: true\n", 1)), 0o644)
	again, _ := Open(dir, publicKinds)
	if d := again.Wiki()["order-items"]; d == nil || len(d.Extra) != 0 {
		t.Errorf("an older key flag: %+v", d)
	}
}
