package featurewiki

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC)

const reserveDoc = `---
id: reserve-stock
name: Reserve stock
description: Hold a quantity of a SKU for an order when enough is available.
interfaces: [Stock.reserve]
related: ["[[check-availability]]"]
updated: 2026-10-22T05:57:04Z
described_commit: c69dd49
described_at: 2026-10-22T05:57:09Z
owner-note: keep
---

## Purpose

An order must not promise stock that another order holds. A held quantity returns through [[release-stock|releasing]].

## Notes

The user wrote this.
`

func body(purpose string) *string {
	s := "## Purpose\n\n" + purpose + "\n"
	return &s
}

func small() Wiki {
	w, _, err := Apply(Wiki{}, []Write{
		{Op: "create", ID: "reserve-stock", Name: "Reserve stock", Description: "Hold stock for an order.", Interfaces: []string{"Stock.reserve"}, Body: body("Holds stock. See [[release-stock]].")},
		{Op: "create", ID: "release-stock", Name: "Release stock", Description: "Return held stock.", Body: body("Returns stock.")},
		{Op: "create", ID: "check-availability", Name: "Check availability", Description: "Report free stock.", Related: []string{"reserve-stock"}, Body: body("Reports stock.")},
	}, Check{}, now)
	if err != nil {
		panic(err)
	}
	return w
}

func TestParseAndRender(t *testing.T) {
	d, err := Parse(reserveDoc)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "reserve-stock" || d.Name != "Reserve stock" || !slices.Equal(d.Interfaces, []string{"Stock.reserve"}) {
		t.Errorf("header: %+v", d)
	}
	if !slices.Equal(d.Related, []string{"check-availability"}) {
		t.Errorf("related links are read as ids: %v", d.Related)
	}
	if d.DescribedCommit != "c69dd49" || d.Updated.Hour() != 5 {
		t.Errorf("code-owned properties: %+v", d)
	}
	if len(d.Extra) != 1 || d.Extra[0].Key != "owner-note" {
		t.Errorf("W6: the user's property is kept: %+v", d.Extra)
	}
	if !slices.Equal(d.Neighbors(), []string{"release-stock", "check-availability"}) {
		t.Errorf("neighbors from the body and related: %v", d.Neighbors())
	}
	if d.Render() != reserveDoc {
		t.Errorf("render differs from the source:\n%s", d.Render())
	}
}

func TestParseRefusals(t *testing.T) {
	for name, text := range map[string]string{
		"no header":    "## Purpose\n",
		"no close":     "---\nid: x\n",
		"bad yaml":     "---\nid: [\n---\n",
		"heading link": "---\nid: x\nrelated: [\"[[y#top]]\"]\n---\n",
	} {
		if _, err := Parse(text); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLinks(t *testing.T) {
	ids, err := Links("See [[a]], [[b|the b]] and [[ c ]].")
	if err != nil || !slices.Equal(ids, []string{"a", "b", "c"}) {
		t.Errorf("links = %v, %v", ids, err)
	}
	if _, err := Links("[[a#heading]]"); err == nil || !strings.Contains(err.Error(), "K1") {
		t.Errorf("K1: a heading link must fail: %v", err)
	}
}

func TestValidateRules(t *testing.T) {
	ok := small()
	if err := ok.Validate(Check{Interfaces: map[string]bool{"Stock.reserve": true}}); err != nil {
		t.Fatalf("a valid wiki: %v", err)
	}
	cases := map[string]struct {
		edit func(Wiki)
		c    Check
	}{
		"W1 id":        {func(w Wiki) { w["Bad"] = &Document{ID: "Bad", Name: "x", Description: "y", Body: *body("p")} }, Check{}},
		"W1 taken":     {func(Wiki) {}, Check{Taken: map[string]bool{"reserve-stock": true}}},
		"W1 file name": {func(w Wiki) { w["reserve-stock"].ID = "other" }, Check{}},
		"W2 name":      {func(w Wiki) { w["reserve-stock"].Name = " " }, Check{}},
		"W2 long":      {func(w Wiki) { w["reserve-stock"].Description = strings.Repeat("x", 201) }, Check{}},
		"W2 lines":     {func(w Wiki) { w["reserve-stock"].Description = "a\nb" }, Check{}},
		"W3":           {func(Wiki) {}, Check{Interfaces: map[string]bool{}}},
		"W4 missing":   {func(w Wiki) { w["reserve-stock"].Body = "## Other\ntext\n" }, Check{}},
		"W4 empty":     {func(w Wiki) { w["reserve-stock"].Body = "## Purpose\n\n## Other\ntext\n" }, Check{}},
		"K1":           {func(w Wiki) { w["reserve-stock"].Body += "\n[[release-stock#x]]\n" }, Check{}},
		"K2 self":      {func(w Wiki) { w["reserve-stock"].Related = []string{"reserve-stock"} }, Check{}},
		"K2 unknown":   {func(w Wiki) { w["reserve-stock"].Body += "\n[[nothing]]\n" }, Check{}},
	}
	for name, tc := range cases {
		w := small()
		tc.edit(w)
		err := w.Validate(tc.c)
		rule := strings.Fields(name)[0]
		if err == nil || !strings.Contains(err.Error(), rule+":") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestNeighborsAreUndirected(t *testing.T) {
	n := small().Neighbors()
	want := map[string][]string{
		"reserve-stock":      {"check-availability", "release-stock"},
		"release-stock":      {"reserve-stock"},
		"check-availability": {"reserve-stock"},
	}
	for id, ns := range want {
		if !slices.Equal(n[id], ns) {
			t.Errorf("K3: neighbors of %s = %v, want %v", id, n[id], ns)
		}
	}
}

func TestApplyRefusesAsAWhole(t *testing.T) {
	w := small()
	_, _, err := Apply(w, []Write{
		{Op: "create", ID: "restock", Name: "Restock", Description: "Add stock.", Body: body("Adds.")},
		{Op: "create", ID: "bad", Name: "Bad", Description: "x", Body: body("links [[nowhere]]")},
	}, Check{}, now)
	if err == nil || !strings.Contains(err.Error(), "K2") {
		t.Fatalf("expected K2, got %v", err)
	}
	if w["restock"] != nil {
		t.Error("the wiki changed although the change failed")
	}
	for name, writes := range map[string][]Write{
		"empty":          nil,
		"no id":          {{Op: "create"}},
		"unknown op":     {{Op: "upsert", ID: "x"}},
		"create exists":  {{Op: "create", ID: "reserve-stock", Name: "x", Description: "y", Body: body("p")}},
		"create no body": {{Op: "create", ID: "x", Name: "x", Description: "y"}},
		"notes given":    {{Op: "create", ID: "x", Name: "x", Description: "y", Body: ptr("## Purpose\np\n## Notes\nmine\n")}},
		"modify missing": {{Op: "modify", ID: "nothing", Fields: &Fields{Name: ptr("x")}}},
		"modify empty":   {{Op: "modify", ID: "reserve-stock"}},
		"rename to used": {{Op: "rename", ID: "reserve-stock", To: "release-stock"}},
		"rename no to":   {{Op: "rename", ID: "reserve-stock"}},
		"bad redirect":   {{Op: "remove", ID: "reserve-stock", Redirect: "nothing"}},
	} {
		if _, _, err := Apply(small(), writes, Check{}, now); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	many := make([]Write, MaxWrites+1)
	for i := range many {
		many[i] = Write{Op: "create", ID: "f" + strings.Repeat("a", i%40), Name: "x", Description: "y", Body: body("p")}
	}
	if _, _, err := Apply(Wiki{}, many, Check{}, now); err == nil || !strings.Contains(err.Error(), "W9") {
		t.Errorf("W9: %v", err)
	}
}

func TestModifyKeepsNotesAndSetsUpdated(t *testing.T) {
	w := small()
	w["reserve-stock"].Body += "\n## Notes\n\nThe user wrote this.\n"
	later := now.Add(time.Hour)
	next, res, err := Apply(w, []Write{{Op: "modify", ID: "reserve-stock", Fields: &Fields{Description: ptr("Hold stock for an order; refuse too much.")}, Body: body("New purpose, still [[release-stock]].")}}, Check{}, later)
	if err != nil {
		t.Fatal(err)
	}
	d := next["reserve-stock"]
	if !strings.Contains(d.Body, "New purpose") || !strings.Contains(d.Body, "## Notes\n\nThe user wrote this.") {
		t.Errorf("W4: the notes must survive a modify:\n%s", d.Body)
	}
	if d.Description != "Hold stock for an order; refuse too much." || !d.Updated.Equal(later) {
		t.Errorf("fields and updated: %+v", d)
	}
	if !slices.Equal(res.Written, []string{"reserve-stock"}) || res.Applied[0] != "modified reserve-stock" {
		t.Errorf("result: %+v", res)
	}
	// An empty list clears a list field; the other document is untouched.
	next, _, err = Apply(next, []Write{{Op: "modify", ID: "reserve-stock", Fields: &Fields{Interfaces: &[]string{}}}}, Check{Interfaces: map[string]bool{}}, later)
	if err != nil || len(next["reserve-stock"].Interfaces) != 0 || !next["release-stock"].Updated.Equal(now) {
		t.Errorf("clearing a list: %v %+v", err, next["reserve-stock"])
	}
}

func TestRenameRewritesLinks(t *testing.T) {
	later := now.Add(time.Hour)
	next, res, err := Apply(small(), []Write{{Op: "rename", ID: "release-stock", To: "free-stock"}}, Check{}, later)
	if err != nil {
		t.Fatal(err)
	}
	if next["release-stock"] != nil || next["free-stock"] == nil || next["free-stock"].ID != "free-stock" {
		t.Fatalf("K4: the document moved: %v", next.IDs())
	}
	if !strings.Contains(next["reserve-stock"].Body, "[[free-stock]]") {
		t.Errorf("K4: links follow a rename:\n%s", next["reserve-stock"].Body)
	}
	if !next["reserve-stock"].Updated.Equal(later) || !next["check-availability"].Updated.Equal(now) {
		t.Error("E2: only documents whose links changed get a new updated")
	}
	if res.Renamed["release-stock"] != "free-stock" || !slices.Equal(res.Written, []string{"free-stock", "reserve-stock"}) {
		t.Errorf("result: %+v", res)
	}
	// A rename of a renamed id in one change points the first old id at the final one.
	_, res, err = Apply(small(), []Write{{Op: "rename", ID: "release-stock", To: "a"}, {Op: "rename", ID: "a", To: "b"}}, Check{}, later)
	if err != nil || res.Renamed["release-stock"] != "b" {
		t.Errorf("chained rename: %v %v", err, res.Renamed)
	}
}

func TestRemoveRedirectsOrUnlinks(t *testing.T) {
	w := small()
	w["reserve-stock"].Body = "## Purpose\n\nSee [[release-stock|releasing]] and [[check-availability]].\n"
	next, res, err := Apply(w, []Write{{Op: "remove", ID: "release-stock"}}, Check{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if next["release-stock"] != nil || !strings.Contains(next["reserve-stock"].Body, "See releasing and [[check-availability]].") {
		t.Errorf("K4: a removed link becomes its text:\n%s", next["reserve-stock"].Body)
	}
	if !slices.Equal(res.Removed, []string{"release-stock"}) {
		t.Errorf("result: %+v", res)
	}
	next, _, err = Apply(w, []Write{{Op: "remove", ID: "release-stock", Redirect: "check-availability"}}, Check{}, now)
	if err != nil || !strings.Contains(next["reserve-stock"].Body, "[[check-availability|releasing]]") {
		t.Errorf("K4: a redirect keeps the text: %v\n%s", err, next["reserve-stock"].Body)
	}
	// A related entry to a removed feature goes, or follows the redirect.
	next, _, err = Apply(w, []Write{{Op: "remove", ID: "reserve-stock", Redirect: "release-stock"}}, Check{}, now)
	if err != nil || !slices.Equal(next["check-availability"].Related, []string{"release-stock"}) {
		t.Errorf("K4: related follows the redirect: %v %v", err, next["check-availability"].Related)
	}
	next, _, err = Apply(w, []Write{{Op: "remove", ID: "reserve-stock"}}, Check{}, now)
	if err != nil || len(next["check-availability"].Related) != 0 {
		t.Errorf("K4: related drops a removed feature: %v %v", err, next["check-availability"].Related)
	}
}

func TestWarnings(t *testing.T) {
	w := small()
	w["lonely"] = &Document{ID: "lonely", Name: "Lonely", Description: "Alone.", Body: "## Purpose\n\nIt reads orders/create.py and nothing else.\n"}
	exists := func(rel string) bool { return rel == "orders/create.py" }
	got := w.Warnings(Check{Exists: exists})
	for _, want := range []string{`"lonely" has no neighbor`, "names the path orders/create.py"} {
		if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, want) }) {
			t.Errorf("warnings lack %q: %v", want, got)
		}
	}
	if len(small().Warnings(Check{Exists: exists})) != 0 {
		t.Errorf("a linked wiki without paths has no warnings: %v", small().Warnings(Check{Exists: exists}))
	}
	big := Wiki{}
	for i := 0; i <= ManyFeatures; i++ {
		id := "f" + strings.Repeat("a", i)
		big[id] = &Document{ID: id, Name: "x", Description: "y", Body: *body("p [[f]]")}
	}
	if got := big.Warnings(Check{}); !strings.Contains(strings.Join(got, "\n"), "listing parts") {
		t.Errorf("many features warn: %v", got)
	}
}

func TestStoreWritesReloadsAndKeepsTheLastValidWiki(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "features")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Change([]Write{
		{Op: "create", ID: "reserve-stock", Name: "Reserve stock", Description: "Hold stock.", Body: body("Holds [[release-stock]].")},
		{Op: "create", ID: "release-stock", Name: "Release stock", Description: "Return stock.", Body: body("Returns.")},
	}, Check{}, now)
	if err != nil || len(res.Applied) != 2 {
		t.Fatalf("change: %v %+v", err, res)
	}
	data, err := os.ReadFile(filepath.Join(dir, "reserve-stock.md"))
	if err != nil || !strings.HasPrefix(string(data), "---\nid: reserve-stock\n") || s.Raw("reserve-stock") != string(data) {
		t.Errorf("W7: the file: %v\n%s", err, data)
	}
	if err := s.SetDescribed("abc123", now); err != nil || s.Wiki()["release-stock"].DescribedCommit != "abc123" {
		t.Errorf("S22: described: %v", err)
	}
	if _, err := s.Change([]Write{{Op: "rename", ID: "release-stock", To: "free-stock"}}, Check{}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "release-stock.md")); err == nil {
		t.Error("a rename removes the old file")
	}
	if len(s.Files()) != 2 || s.Files()["free-stock.md"] == nil {
		t.Errorf("files: %v", s.Files())
	}
	if s.Reload() {
		t.Error("nothing changed on disk")
	}

	// The user edits a document by hand: a valid edit is accepted as it is.
	edited := strings.Replace(s.Raw("free-stock"), "## Purpose\n", "## Purpose\n\nEdited by hand.\n", 1) + "\n## Notes\n\nMine.\n"
	os.WriteFile(filepath.Join(dir, "free-stock.md"), []byte(edited), 0o644)
	if !s.Reload() || !strings.Contains(s.Wiki()["free-stock"].Body, "Edited by hand") || s.Error() != "" {
		t.Errorf("W8: a valid hand edit is read: %q", s.Error())
	}
	// An invalid edit keeps the last valid wiki and reports the error.
	os.WriteFile(filepath.Join(dir, "free-stock.md"), []byte("---\nid: other\n---\n"), 0o644)
	if !s.Reload() || s.Error() == "" || s.Wiki()["free-stock"] == nil {
		t.Errorf("W8: an invalid edit keeps the last wiki; error %q", s.Error())
	}
	// Reopening an invalid directory fails with the problem.
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "W1") {
		t.Errorf("open an invalid wiki: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
