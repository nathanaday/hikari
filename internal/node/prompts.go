package node

import (
	"fmt"
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
	b.WriteString(`- Keep your system map true. It is how your orchestrator learns what your system does; the orchestrator never reads your code. The map tools are mcp__hikari__get_my_system, set_system_info, upsert_features, remove_feature, record_test_results, and mark_map_current.
- Do not try to change the .hikari directory in your scope. It is read-only to you; the map tools change it.
- For domain terms and interface names, use the names in the vocabularies of your hubs (mcp__hikari__get_vocabulary).
`)
	fmt.Fprintf(&b, "- After each unit of work, call mcp__hikari__commit_changes with a message that says what changed and why: a subject line and an optional body, with no trailers such as Co-Authored-By, because hikari sets the author. Commits go to the branch %q. hikari also commits what you leave at the end of each turn.\n", s.Branch)
	test := "the test command in your map"
	if s.TestCommand != "" {
		test = fmt.Sprintf("`%s`", s.TestCommand)
	}
	fmt.Fprintf(&b, "- After a change, run the tests of the affected features with %s, record the results, and update the affected features. Then call mark_map_current.\n", test)
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
Update the system info and the features that these changes affect. Run the tests of the affected features and record the results. Then call mark_map_current.
`)
	} else {
		b.WriteString(`Your system map is not built yet. Build it now:

1. Read the code in your scope to learn what the system does. Read what you need, not every file.
2. Call set_system_info with a summary, the responsibilities, the interfaces that the system provides and consumes, a layout of its main parts, and the test command.
3. Define 5 to 30 features with upsert_features, in one or a few calls. A feature is something that a user or another system relies on. List its code and test paths, relative to your scope.
4. Run the tests, and call record_test_results for each feature that has tests.
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
			notes = append(notes, "Your system map is not built yet. Before you end this turn, describe your system with the map tools and call mark_map_current.")
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
