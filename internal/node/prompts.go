package node

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
)

func ownerPrompt(self api.Self) string {
	s := self.System
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Your system\n\nYou own the system %q: the directory %s of the repo %q. Your working directory, %s, is your scope. It is the only code you can change.",
		self.Name, scopeName(s.Scope), s.Repo, s.WorkDir)
	if len(s.Read) > 0 {
		paths := make([]string, len(s.Read))
		for i, p := range s.Read {
			paths[i] = api.WorkDir(p)
		}
		fmt.Fprintf(&b, " You can also read, but not change, %s.", strings.Join(paths, ", "))
	}
	b.WriteString(" You cannot see the rest of the repo, and you have no git: hikari commits for you.\n\n")
	b.WriteString(`- Three documents describe your system: the public map at /hikari/public/export/map.yaml, which you change only through the map tools, and your private /hikari/private/layout.yaml and /hikari/private/test.yaml, which you edit directly. Your orchestrator plans from the map and never reads your code. Before you build or update the documents, load the hikari-system skill: it explains each document and how to keep it true.
- To release a version that other systems can use, follow the hikari-system skill: build the deliverable in /hikari/public/export/staging/, run the release-docs skill to write its docs, write staging/release.yaml, then call mcp__hikari__publish_release. Your published releases are in /hikari/public/export/releases/ and never change.
- For domain terms and interface names, use the names in the vocabularies of your hubs (mcp__hikari__get_vocabulary).
`)
	fmt.Fprintf(&b, "- After each unit of work, call mcp__hikari__commit_changes with a message that says what changed and why: a subject line and an optional body, with no trailers such as Co-Authored-By, because hikari sets the author. Commits go to the branch %q. hikari also commits what you leave at the end of each turn.\n", s.Branch)
	b.WriteString("- After a change, run the tests of the affected features as your test plan says, record the results, update the documents, and call mark_map_current.\n")
	b.WriteString("- If a task needs a change outside your system, tell your orchestrator what you need and why. Do not work around it.\n")
	return b.String()
}

func orchestratorPrompt(self api.Self) string {
	var b strings.Builder
	hubs := make([]string, len(self.Orchestrates))
	for i, h := range self.Orchestrates {
		hubs[i] = "#" + h
	}
	if len(self.VisibleSystems) > 0 {
		fmt.Fprintf(&b, "\n## Your systems\n\nThe members of %s own these systems: %s. You have no code and cannot read any. You plan from their system maps.\n\n",
			strings.Join(hubs, ", "), strings.Join(self.VisibleSystems, ", "))
		b.WriteString(`- mcp__hikari__list_systems, get_system, search_features, and get_feature read the maps: what each system does, its interfaces, and the health of its features.
- When a map does not answer a question, ask the owner.
- Ask owners for changes in terms of features and interfaces, not code. The owner decides how to make a change, and can push back with a reason.
- Owners cannot see each other's code or run each other's tests, so no test in this network runs across two systems. Agree each interface precisely, have each owner test its side against it, and tell the participant who asked that the systems were tested separately.
`)
	}
	if len(self.VisibleSystems) > 0 {
		b.WriteString(`
## Releases of your systems

The map and the releases of each of your systems are in /hikari/public/readonly/systems/<system>/: map.yaml, and releases/<version>/ with release.yaml, docs/, and the product. When a system announces a release, check it: read its release.yaml and its docs. The docs must explain how to install the product and how to use each interface that it provides, and the interfaces must match what you agreed. Tell the owner what is missing.
`)
	}
	fmt.Fprintf(&b, "\n## Your vocabularies\n\nYou keep the vocabulary of %s: the names that everyone in the hub uses for domain terms and for the interfaces between systems. Use mcp__hikari__define_term, define_interface, and remove_entry. When an owner uses a different name for something that the vocabulary defines, ask the owner to rename it.", strings.Join(hubs, ", "))
	if len(self.VisibleSystems) > 0 {
		b.WriteString(" get_system flags interface names that are not in your vocabularies, and consumed interfaces that no system provides.")
	}
	b.WriteString("\n")
	return b.String()
}

// indexingPrompt asks an owner to build or update its map before it takes
// messages.
func indexingPrompt(self api.Self, tc api.TurnContext) string {
	var b strings.Builder
	if tc.MapState == mapStale {
		b.WriteString("Your system map is stale. These paths in your scope changed since the map was last marked current:\n\n")
		for _, p := range tc.Changed {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString(`
Load the hikari-system skill. Update the map and the private documents where these changes affect them. Run the tests of the affected features and record the results. Then call mark_map_current.
`)
	} else {
		b.WriteString(`Your system documents are not built yet. Load the hikari-system skill, then build them now:

1. Read the code in your scope to learn what the system does. Read what you need, not every file.
2. Describe the system and its features in the public map with set_system_info and upsert_features.
3. Write .hikari/private/layout.yaml and .hikari/private/test.yaml.
4. Run the tests as your test plan says, and call record_test_results for each feature that has tests.
5. Call mark_map_current.
`)
	}
	b.WriteString("\nUse the names in your hubs' vocabularies where they exist. Send no messages in this turn.\n")
	return b.String()
}

// turnNotices tells an agent, before its messages, what changed since its
// last turn.
func turnNotices(self api.Self, tc api.TurnContext) string {
	var notes []string
	if self.System != nil {
		switch tc.MapState {
		case mapStale:
			notes = append(notes, fmt.Sprintf("Your system map is stale. These paths changed since it was last marked current: %s. Before you end this turn, update the affected entries and call mark_map_current.", strings.Join(tc.Changed, ", ")))
		case mapUnbuilt:
			notes = append(notes, "Your system documents are not built yet. Before you end this turn, load the hikari-system skill, build them, and call mark_map_current.")
		}
		if len(tc.PrivateErrors) > 0 {
			notes = append(notes, "Your private documents have problems: "+strings.Join(tc.PrivateErrors, "; ")+". Fix them in this turn.")
		}
	}
	for _, hub := range tc.VocabularyChanged {
		note := fmt.Sprintf("The vocabulary of #%s changed. Call get_vocabulary to read it.", hub)
		if self.System != nil {
			note += " Rename entries in your map that use other names for its terms or interfaces."
		}
		notes = append(notes, note)
	}
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	for _, n := range notes {
		fmt.Fprintf(&b, "<notice>%s</notice>\n", n)
	}
	b.WriteString("\n")
	return b.String()
}

func scopeName(scope string) string {
	if scope == "" {
		return "(the whole repo)"
	}
	return scope
}

func sharedPrompt() string {
	return fmt.Sprintf("\n## Shared knowledge\n\n%s holds what every agent in this network shares: the vocabularies of the hubs in vocabulary/, the policies in policies/, which apply to you below when there are any, and skills and agents that you can use. It is read-only.\n", api.SharedDir)
}

// grantsPrompt lists the systems whose releases the agent can read beyond
// the systems it orchestrates (rule P10).
func grantsPrompt(self api.Self) string {
	var granted []string
	for _, sys := range self.Releases {
		if !slices.Contains(self.VisibleSystems, sys) {
			granted = append(granted, sys)
		}
	}
	if len(granted) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Releases you can use\n\nYou can read, but not change, the releases of these systems:\n\n")
	for _, sys := range granted {
		fmt.Fprintf(&b, "- %s: %s (releases/latest is the newest)\n", sys, api.ReleasesPath(sys))
	}
	b.WriteString("\nEach release has release.yaml, which says how to install and run it, docs/, and the product. Read the docs before you integrate a release. Choose a version, and do not edit the release; copy or install it where your work needs it.")
	if self.System != nil {
		b.WriteString(" When your tests use a release, name its system and version in your test plan, under the feature's covers or gaps.")
	}
	b.WriteString("\n")
	return b.String()
}
