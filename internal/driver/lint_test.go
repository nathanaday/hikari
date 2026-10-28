package driver

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
)

// retired are the words that the glossary retired (rule N11 of the
// shared-language-names spec), each with the word that replaced it.
var retired = []struct {
	word *regexp.Regexp
	now  string
}{
	{regexp.MustCompile(`(?i)\bhubs?\b`), "the room"},
	{regexp.MustCompile(`(?i)\blead\b`), "the orchestration agent"},
	{regexp.MustCompile(`(?i)\bstores?\b`), "the vault"},
	{regexp.MustCompile(`(?i)\bsystem maps?\b`), "the feature map"},
	{regexp.MustCompile(`(?i)\bowners?\b`), "the partition agent"},
	{regexp.MustCompile(`(?i)\bnodes?\b`), "the driver, or the agent"},
	{regexp.MustCompile(`(?i)\bcompose files?\b`), "the topology file"},
	{regexp.MustCompile(`(?i)\bstor(y|ies)\b`), "an orchestration feature"},
}

// allowed lists each place where a retired word stays, with its reason.
var allowed = []struct{ file, text, reason string }{
	{"", "Docker Compose file", "Docker Compose keeps its own name"},
	{"", "store.ts", "the UI's reducer, a standard name in React (decision 6)"},
	{"hikify/references/synthesis.md", "the cloud stores", "a verb in an example"},
	{"hikify/references/analysis.md", "Compose files, Dockerfiles", "the Docker Compose files of a repository"},
	{"components/graph-canvas.tsx", "node", "a node of the drawn graph"},
	{"components/graph-engine.ts", "node", "a node of the drawn graph"},
	{"components/map-view.tsx", "node", "a node of the drawn graph"},
}

// TestRetiredWords keeps the retired words out of the prompts, the skills,
// the agent definitions, the host skills, and the UI's text.
func TestRetiredWords(t *testing.T) {
	texts := map[string]string{}
	for name, self := range lintSelves() {
		texts["prompt of "+name] = systemPrompt(self) + indexingPrompt(self, api.TurnContext{}) +
			indexingPrompt(self, api.TurnContext{MapState: mapStale, NoLocalResults: true}) +
			restorePrompt(api.TurnContext{NoLocalResults: true}) +
			turnNotices(self, api.TurnContext{MapState: mapStale, VocabularyChanged: true, WikiChanged: []api.WikiChange{{Wiki: "public", Docs: []api.DocChange{{ID: "x", Kind: "rule"}}}}})
	}
	fs.WalkDir(builtin, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := builtin.ReadFile(path)
			texts["internal/driver/"+path] = string(data)
		}
		return err
	})
	readTree(t, "../../plugin/skills", func(path, text string) { texts["plugin/skills/"+path] = text })
	readTree(t, "../../web/src/components", func(path, text string) {
		if strings.Contains(path, ".test.") || filepath.Dir(path) == "ui" {
			return // tests assert text, and ui/ holds the vendored components
		}
		texts["components/"+path] = uiText(text)
	})
	for file, text := range texts {
		for _, a := range allowed {
			if a.file == "" || strings.HasSuffix(file, a.file) {
				text = strings.ReplaceAll(text, a.text, "")
			}
		}
		for _, r := range retired {
			for _, w := range r.word.FindAllString(text, -1) {
				t.Errorf("N11: %s says %q; the glossary says %s", file, w, r.now)
			}
		}
	}
}

// lintSelves returns an agent of each role, with every section of its
// system prompt.
func lintSelves() map[string]api.Self {
	room := []api.Peer{
		{Name: "conductor", Role: api.RoleOrchestration},
		{Name: "orders", Role: api.RolePartition},
		{Name: "inventory", Role: api.RolePartition},
		{Name: "user", Role: api.RoleOperator},
	}
	deliverable := []api.DeliverableInfo{{Name: "python-library", Description: "A package."}}
	return map[string]api.Self{
		"the orchestration agent": {
			Name: "conductor", Orchestration: true, Participants: room,
			VisiblePartitions: []string{"inventory", "orders"}, Releases: []string{"inventory", "orders"},
			PartitionAgentGrants:       map[string][]string{"orders": {"inventory"}},
			PartitionAgentDeliverables: map[string][]api.DeliverableInfo{"inventory": deliverable},
		},
		"the orchestration agent of an empty project": {Name: "conductor", Orchestration: true, Participants: room},
		"a partition agent": {
			Name: "orders", Participants: room, Releases: []string{"inventory"}, Deliverables: deliverable, Skills: []string{"style"},
			Partition: &api.SelfPartition{
				Folders: []api.Folder{{Place: "shop/services/orders", Repo: "shop", Path: "services/orders"}, {Place: "shop/proto", Repo: "shop", Path: "proto", ReadOnly: true}},
				Repos:   []api.RepoState{{Name: "shop", Branch: "hikari"}}, WorkDir: "/workspace",
			},
		},
		"a partition agent with no orchestration agent": {
			Name: "orders", Participants: room[1:],
			Partition: &api.SelfPartition{Folders: []api.Folder{{Place: "shop", Repo: "shop"}}, Repos: []api.RepoState{{Name: "shop", Branch: "hikari"}}, WorkDir: "/workspace"},
		},
	}
}

func readTree(t *testing.T, root string, fn func(path, text string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		fn(filepath.ToSlash(rel), string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var (
	tsComment = regexp.MustCompile(`(?s:/\*.*?\*/)|(?m:^\s*//.*$)|(?m:\s//[^'"` + "`" + `\n]*$)`)
	tsImport  = regexp.MustCompile(`(?m)^import .*$`)
	tsString  = regexp.MustCompile(`'(?:[^'\\\n]|\\.)*'|"(?:[^"\\\n]|\\.)*"|` + "`(?:[^`\\\\]|\\\\.)*`")
	jsxText   = regexp.MustCompile(`[>}]([^<>{}]+)[<{]`)
)

// uiText returns the string literals and the JSX text of a component's
// source: what the UI can show.
func uiText(src string) string {
	src = tsImport.ReplaceAllString(tsComment.ReplaceAllString(src, ""), "")
	var b strings.Builder
	for _, s := range tsString.FindAllString(src, -1) {
		b.WriteString(s + "\n")
	}
	for _, m := range jsxText.FindAllStringSubmatch(src, -1) {
		b.WriteString(m[1] + "\n")
	}
	return b.String()
}
