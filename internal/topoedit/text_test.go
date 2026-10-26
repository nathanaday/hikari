package topoedit

import (
	"strings"
	"testing"
)

const sample = `# The team.
name: team

x-hikari:
  # Agents commit here.
  project: {branch: "${HIKARI_BRANCH:-hikari}"}
  hubs:
    product:
      description: Product planning.
      orchestrator: orchestrator
      members: [writer, eng]   # the writers

    eng: {orchestrator: eng, members: [dev]}

x-agent: &agent
  image: hikari-agent

services:
  orchestrator:
    <<: *agent
    x-hikari:
      description: Orchestrates.
      instructions: |
        Plan first.
        Then ask.

  writer:
    <<: *agent
    x-hikari: {description: Writes.}
  eng:
    <<: *agent
    x-hikari:
      description: Engineering.
  dev:
    <<: *agent
    x-hikari:
      description: Developer.
      system:
        repo: app
        read:
          - proto
          - docs
`

func mustParse(t *testing.T, text string) *doc {
	t.Helper()
	d, err := parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// changedLines returns the lines of before that after does not keep, and
// the lines of after that are new, by a line diff.
func changedLines(before, after string) (removed, added []string) {
	for _, l := range diffLines(before, after) {
		switch l.Op {
		case "-":
			removed = append(removed, l.Text)
		case "+":
			added = append(added, l.Text)
		}
	}
	return removed, added
}

func TestPrimitives(t *testing.T) {
	cases := []struct {
		name    string
		edit    func(d *doc) ([]byte, error)
		removed []string
		added   []string
	}{
		{"set a block scalar", func(d *doc) ([]byte, error) {
			return d.setValue(parsePath("services.eng.x-hikari"), "description", scalar("Builds it."), false)
		}, []string{"      description: Engineering."}, []string{"      description: Builds it."}},
		{"set a key in a flow mapping", func(d *doc) ([]byte, error) {
			return d.setValue(parsePath("services.writer.x-hikari"), "name", scalar("scribe"), true)
		}, []string{"    x-hikari: {description: Writes.}"}, []string{"    x-hikari: {name: scribe, description: Writes.}"}},
		{"add a key first, above the comment", func(d *doc) ([]byte, error) {
			return d.setValue(parsePath("x-hikari"), "name", scalar("Team"), true)
		}, nil, []string{"  name: Team"}},
		{"add a key last to a block mapping", func(d *doc) ([]byte, error) {
			return d.setValue(parsePath("services.eng.x-hikari"), "model", scalar("haiku"), false)
		}, nil, []string{"      model: haiku"}},
		{"replace a literal block", func(d *doc) ([]byte, error) {
			return d.setValue(parsePath("services.orchestrator.x-hikari"), "instructions", scalar("One line."), false)
		}, []string{"      instructions: |", "        Plan first.", "        Then ask."}, []string{"      instructions: One line."}},
		{"write a multi-line value as a literal block", func(d *doc) ([]byte, error) {
			return d.setValue(parsePath("services.eng.x-hikari"), "instructions", scalar("A.\nB.\n"), false)
		}, nil, []string{"      instructions: |", "        A.", "        B."}},
		{"delete a block key", func(d *doc) ([]byte, error) {
			return d.deleteKey(parsePath("services.orchestrator.x-hikari"), "instructions")
		}, []string{"      instructions: |", "        Plan first.", "        Then ask."}, nil},
		{"delete a flow key", func(d *doc) ([]byte, error) {
			return d.deleteKey(parsePath("x-hikari.hubs.eng"), "members")
		}, []string{"    eng: {orchestrator: eng, members: [dev]}"}, []string{"    eng: {orchestrator: eng}"}},
		{"delete the last key of a block mapping", func(d *doc) ([]byte, error) {
			return d.deleteKey(parsePath("services.eng.x-hikari"), "description")
		}, []string{"    x-hikari:", "      description: Engineering."}, []string{"    x-hikari: {}"}},
		{"rename a key", func(d *doc) ([]byte, error) {
			return d.renameKey(parsePath("x-hikari.hubs"), "eng", "platform")
		}, []string{"    eng: {orchestrator: eng, members: [dev]}"}, []string{"    platform: {orchestrator: eng, members: [dev]}"}},
		{"append to a flow list keeps the comment", func(d *doc) ([]byte, error) {
			return d.appendItem(parsePath("x-hikari.hubs.product.members"), scalar("dev"))
		}, []string{"      members: [writer, eng]   # the writers"}, []string{"      members: [writer, eng, dev]   # the writers"}},
		{"append to a block list", func(d *doc) ([]byte, error) {
			return d.appendItem(parsePath("services.dev.x-hikari.system.read"), scalar("api"))
		}, nil, []string{"          - api"}},
		{"append to a missing list", func(d *doc) ([]byte, error) {
			return d.appendItem(parsePath("services.eng.x-hikari.grants"), scalar("x"))
		}, nil, []string{"      grants: [x]"}},
		{"remove from a flow list", func(d *doc) ([]byte, error) {
			return d.removeItem(parsePath("x-hikari.hubs.product.members"), 0)
		}, []string{"      members: [writer, eng]   # the writers"}, []string{"      members: [eng]   # the writers"}},
		{"remove from a block list", func(d *doc) ([]byte, error) {
			return d.removeItem(parsePath("services.dev.x-hikari.system.read"), 0)
		}, []string{"          - proto"}, nil},
		{"set a list item", func(d *doc) ([]byte, error) {
			return d.setItem(parsePath("x-hikari.hubs.eng.members[0]"), scalar("builder"))
		}, []string{"    eng: {orchestrator: eng, members: [dev]}"}, []string{"    eng: {orchestrator: eng, members: [builder]}"}},
		{"a value that needs quotes", func(d *doc) ([]byte, error) {
			return d.setValue(parsePath("services.eng.x-hikari"), "description", scalar("yes: really"), false)
		}, []string{"      description: Engineering."}, []string{`      description: 'yes: really'`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := c.edit(mustParse(t, sample))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parse(out); err != nil {
				t.Fatalf("the result does not parse: %v\n%s", err, out)
			}
			removed, added := changedLines(sample, string(out))
			if strings.Join(removed, "\n") != strings.Join(c.removed, "\n") || strings.Join(added, "\n") != strings.Join(c.added, "\n") {
				t.Errorf("TE1: removed %q\nadded %q\nwant removed %q\nadded %q\n%s", removed, added, c.removed, c.added, out)
			}
		})
	}
}

func TestMergeKeyIsKept(t *testing.T) {
	out, err := mustParse(t, sample).setValue(parsePath("x-hikari.hubs"), "ops", mapping("orchestrator", "eng"), false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "!!merge") || !strings.Contains(string(out), "    ops: {orchestrator: eng}\n") {
		t.Errorf("%s", out)
	}
}
