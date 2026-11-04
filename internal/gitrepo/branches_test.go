package gitrepo

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestBranchesAndDefault(t *testing.T) {
	url := bareRemote(t, shop)
	r, err := Clone(url, filepath.Join(t.TempDir(), "shop"))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.DefaultBranch(Origin); got != "main" {
		t.Errorf("DefaultBranch = %q, want main", got)
	}
	if _, err := r.Prepare("hikari"); err != nil {
		t.Fatal(err)
	}
	write(t, r.Path, "a.txt", "a\n")
	if _, err := r.CommitPaths("hikari", []string{"a.txt"}, "orders", "orders: a"); err != nil {
		t.Fatal(err)
	}
	local, err := r.Branches()
	if err != nil {
		t.Fatal(err)
	}
	names := func(refs []Ref) []string {
		var out []string
		for _, x := range refs {
			out = append(out, x.Name)
		}
		slices.Sort(out)
		return out
	}
	if got := names(local); !slices.Equal(got, []string{"hikari", "main"}) {
		t.Errorf("Branches = %v", got)
	}
	for _, b := range local {
		if b.Name == "main" && b.Upstream != "origin/main" {
			t.Errorf("main's upstream = %q", b.Upstream)
		}
		if b.Name == "hikari" && b.Upstream != "" {
			t.Errorf("hikari's upstream = %q, want none before the push", b.Upstream)
		}
	}
	remote, err := r.RemoteBranches(Origin)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(remote); !slices.Equal(got, []string{"main"}) {
		t.Errorf("RemoteBranches = %v", got)
	}
	ahead, behind, err := r.AheadBehind("refs/heads/hikari", "refs/heads/main")
	if err != nil || ahead != 1 || behind != 0 {
		t.Errorf("AheadBehind = %d, %d, %v; want 1, 0", ahead, behind, err)
	}
	if r.IsAncestor("hikari", "main") {
		t.Error("hikari is not merged into main")
	}
	if !r.IsAncestor("main", "hikari") {
		t.Error("main is an ancestor of hikari")
	}
	c, err := r.LastCommit("hikari")
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "orders: a" || c.Author != "orders (hikari)" || c.SHA == "" || c.Time.IsZero() {
		t.Errorf("LastCommit = %+v", c)
	}
	if !r.HasRemote(Origin) || r.HasRemote("nope") {
		t.Error("HasRemote")
	}
	if err := r.FetchQuiet(context.Background(), Origin, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := r.FetchQuiet(context.Background(), "nope", time.Second); err == nil {
		t.Error("a fetch of a missing remote fails")
	}
}

func TestDefaultBranchWithoutRemote(t *testing.T) {
	r := newRepo(t, shop)
	if got := r.DefaultBranch(Origin); got != "main" {
		t.Errorf("DefaultBranch = %q, want main from the local branch", got)
	}
	if _, err := r.RemoteBranches(Origin); err != nil {
		t.Errorf("no remote branches is not an error: %v", err)
	}
}
