package node

import (
	"fmt"
	"maps"
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
	b.WriteString(`- Your system's wiki is /hikari/public/export/wiki/: one markdown document for each feature, principle, and rule, with links between the documents that work together, and the user's plans and notes. You read the files and change your documents only with mcp__hikari__change_wiki. hikari writes the public map, /hikari/public/export/map.yaml, from the features and from set_system_info. Your private /hikari/private/layout.yaml, the code map that says where each feature lives, and /hikari/private/tests.yaml, the test map that says how your suites run and which features each test touches, are yours alone; you edit them directly. Your orchestrator plans from the feature documents and the map and never reads your code. Before you build or change the documents, load the hikari-system skill: it explains each document, what counts as a feature, a principle, or a rule, how to link, and how to keep them true.
- Releases follow your deliverables, listed below. Your published releases are in /hikari/public/export/releases/ and never change.
- For domain terms and interface names, use the names in the vocabularies of your hubs (mcp__hikari__get_vocabulary).
`)
	fmt.Fprintf(&b, "- After each unit of work, call mcp__hikari__commit_changes with a message that says what changed and why: a subject line and an optional body, with no trailers such as Co-Authored-By, because hikari sets the author. Commits go to the branch %q. hikari also commits what you leave at the end of each turn.\n", s.Branch)
	b.WriteString("- After a change, run the suites that touch the affected features, call mcp__hikari__record_test_run for each run, update the documents, and call mark_map_current. hikari computes each feature's health from the tests that touch it.\n")
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
		b.WriteString(`- mcp__hikari__list_systems and get_system read the maps: what each system does, its interfaces, and the health and state of its features. Each system's wiki is in /hikari/public/readonly/systems/<system>/wiki/: one document for each feature, principle, and rule. get_document returns a document with the documents that link it, and search_wiki searches every wiki you can read.
- When a map does not answer a question, ask the owner.
- Ask owners for changes in terms of features and interfaces, not code. The owner decides how to make a change, and can push back with a reason.
- Owners cannot see each other's code. A system can test against another system only through that system's published releases, and only when the topology grants it those releases. Otherwise, agree each interface precisely, have each owner test its side against it, and tell the participant who asked that the systems were tested separately.
`)
		b.WriteString(memberGrants(self))
		b.WriteString(memberDeliverables(self))
	}
	if len(self.VisibleSystems) > 0 {
		fmt.Fprintf(&b, `
## Releases of your systems

The map and the releases of each of your systems are in /hikari/public/readonly/systems/<system>/: map.yaml, and releases/<deliverable>/<version>/ with release.yaml, SHA256SUMS, product/, and map.yaml. A release is a product of one deliverable: a named package in %s/deliverables/<name>/ that says what the product must contain (deliverable.yaml) and how the owner makes it (SKILL.md). When a system announces a release, check it: read its release.yaml, the product's docs, and the deliverable's definition. The product must meet what the deliverable promises and what the hub agreed, and its docs must explain how to install it and how to use each interface that it provides. Tell the owner what is missing. You cannot change deliverables or who holds them; ask the user.
`, api.SharedDir)
	}
	for _, hub := range slices.Sorted(maps.Keys(self.Testers)) {
		fmt.Fprintf(&b, "\n%s is the tester of #%s: it proves the hub's stories and rules against the releases of its systems, and each story gets health from its tests. mcp__hikari__get_test_plan shows its plan, the gaps that the user approved, and the health of each story, with the systems a failing story suspects.\n", self.Testers[hub], hub)
	}
	fmt.Fprintf(&b, "\n## Your hub wikis\n\nYou keep the wiki of %s: the stories, rules, and principles that say what the hub's systems must do together. Change it only with mcp__hikari__change_hub_wiki, and link each story to the features of the systems that carry it out. Before you write or change a hub wiki, load the hikari-hub skill. A story that the user marked key is part of the integration test that matters most to the user.\n", strings.Join(hubs, ", "))
	fmt.Fprintf(&b, "\n## Your vocabularies\n\nYou keep the vocabulary of %s: the names that everyone in the hub uses for domain terms and for the interfaces between systems. Use mcp__hikari__define_term, define_interface, and remove_entry. When an owner uses a different name for something that the vocabulary defines, ask the owner to rename it.", strings.Join(hubs, ", "))
	if len(self.VisibleSystems) > 0 {
		b.WriteString(" get_system flags interface names that are not in your vocabularies, and consumed interfaces that no system provides.")
	}
	b.WriteString("\n")
	return b.String()
}

// indexingPrompt asks an owner to build or update its documents before it
// takes messages (rule D8).
func indexingPrompt(self api.Self, tc api.TurnContext) string {
	var b strings.Builder
	if self.Tests != "" && tc.MapState != mapStale {
		fmt.Fprintf(&b, "You are the tester of #%s, and your documents are not built yet. Load the hikari-tester skill and follow its Start procedure: read the wiki of #%s and the releases you can read, write %s/%s, pin the releases with pin_releases, describe your scaffold with set_system_info, and call mark_map_current. Then post the plan's summary in #%s and ask the user to review it.\n", self.Tests, self.Tests, self.System.WorkDir, e2ePlan, self.Tests)
		return b.String()
	}
	if tc.MapState == mapStale {
		b.WriteString("Your system documents are stale: the code changed since they were last marked current.\n\n")
		b.WriteString(staleLists(tc))
		b.WriteString(healthLists(tc))
		b.WriteString("\nLoad the hikari-system skill and follow its Keep true procedure: update the stale features, place the unmapped paths, run the suites that touch the affected features and call record_test_run for each run, then call mark_map_current.\n")
	} else {
		b.WriteString(`Your system documents are not built yet. Load the hikari-system skill and follow its Build procedure:

1. Read the code in your scope to learn what the system does, and write the parts of layout.yaml. Read what you need, not every file; send the hikari-feature-survey agent for each large part.
2. Describe the system with set_system_info, then write one feature document for each feature with change_wiki, linked to the features it works with, and a principle or a rule where the code shows one.
3. Write the code map of each feature in /hikari/private/layout.yaml.
4. Write the suites of /hikari/private/tests.yaml, each with a command that writes a JUnit XML report to /hikari/private/reports/<suite>.xml. Run each suite and call record_test_run for it.
5. Map the unmapped tests that get_my_system lists to the features they touch, in tests.yaml; send the hikari-test-survey agent when there are many.
6. Call mark_map_current.
`)
	}
	b.WriteString("\nUse the names in your hubs' vocabularies where they exist. Send no messages in this turn.\n")
	return b.String()
}

// staleLists renders the stale features with their changed paths, the
// undescribed features, and the unmapped paths, as lists.
func staleLists(tc api.TurnContext) string {
	var b strings.Builder
	if len(tc.StaleFeatures) > 0 {
		b.WriteString("Stale features, with the paths that changed:\n")
		for _, f := range tc.StaleFeatures {
			fmt.Fprintf(&b, "- %s: %s\n", f.ID, strings.Join(f.Paths, ", "))
		}
	}
	if len(tc.UndescribedFeatures) > 0 {
		fmt.Fprintf(&b, "Features created since the last mark_map_current: %s.\n", strings.Join(tc.UndescribedFeatures, ", "))
	}
	if len(tc.UnmappedPaths) > 0 {
		b.WriteString("Changed paths that belong to no feature:\n")
		for _, p := range tc.UnmappedPaths {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	return b.String()
}

// Limits of the health notice (rule D10).
const (
	maxNoticeFeatures = 20
	maxNoticeTests    = 5
)

// healthLists renders the failing features with their failing tests and
// the pending features, as lists.
func healthLists(tc api.TurnContext) string {
	var b strings.Builder
	if len(tc.FailingFeatures) > 0 {
		b.WriteString("Failing features, with their failing tests:\n")
		for _, f := range tc.FailingFeatures {
			fmt.Fprintf(&b, "- %s: %s\n", f.ID, capped(f.Tests, maxNoticeTests))
		}
	}
	if len(tc.PendingFeatures) > 0 {
		fmt.Fprintf(&b, "Pending features, whose tests have not run since their code changed: %s.\n", capped(tc.PendingFeatures, maxNoticeFeatures))
	}
	return b.String()
}

// healthSummary renders the failing and pending features in one sentence,
// for a notice (rule D10).
func healthSummary(tc api.TurnContext) string {
	var parts []string
	if n := len(tc.FailingFeatures); n > 0 {
		shown := tc.FailingFeatures[:min(n, maxNoticeFeatures)]
		items := make([]string, len(shown))
		for i, f := range shown {
			items[i] = fmt.Sprintf("%s (%s)", f.ID, capped(f.Tests, maxNoticeTests))
		}
		text := "Failing features: " + strings.Join(items, "; ")
		if n > len(shown) {
			text += fmt.Sprintf("; and %d more", n-len(shown))
		}
		parts = append(parts, text+".")
	}
	if len(tc.PendingFeatures) > 0 {
		parts = append(parts, "Pending features: "+capped(tc.PendingFeatures, maxNoticeFeatures)+".")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + " Run the suites that touch them, call record_test_run for each run, and fix the code or the test map."
}

// capped joins at most n items and counts the others.
func capped(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(items[:n], ", "), len(items)-n)
}

// staleSummary renders the same lists in one sentence, for a notice.
func staleSummary(tc api.TurnContext) string {
	var parts []string
	if len(tc.StaleFeatures) > 0 {
		items := make([]string, len(tc.StaleFeatures))
		for i, f := range tc.StaleFeatures {
			items[i] = fmt.Sprintf("%s (%s)", f.ID, strings.Join(f.Paths, ", "))
		}
		parts = append(parts, "Stale features: "+strings.Join(items, "; ")+".")
	}
	if len(tc.UndescribedFeatures) > 0 {
		parts = append(parts, "Features created since the last mark_map_current: "+strings.Join(tc.UndescribedFeatures, ", ")+".")
	}
	if len(tc.UnmappedPaths) > 0 {
		parts = append(parts, "Changed paths that belong to no feature: "+strings.Join(tc.UnmappedPaths, ", ")+".")
	}
	return strings.Join(parts, " ")
}

// turnNotices tells an agent, before its messages, what changed since its
// last turn (rule D9).
func turnNotices(self api.Self, tc api.TurnContext) string {
	var notes []string
	if self.System != nil {
		switch tc.MapState {
		case mapStale:
			notes = append(notes, "Your system documents are stale. "+staleSummary(tc)+" Before you end this turn, follow the Keep true procedure of the hikari-system skill and call mark_map_current.")
		case mapUnbuilt:
			notes = append(notes, "Your system documents are not built yet. Before you end this turn, load the hikari-system skill, follow its Build procedure, and call mark_map_current.")
		}
		if h := healthSummary(tc); h != "" {
			notes = append(notes, h)
		}
		if len(tc.PrivateErrors) > 0 {
			notes = append(notes, "Your private documents have problems: "+strings.Join(tc.PrivateErrors, "; ")+". Fix them in this turn.")
		}
		if len(tc.NewerReleases) > 0 {
			notes = append(notes, "Newer releases: "+strings.Join(tc.NewerReleases, "; ")+". Follow the Keep true procedure of the hikari-tester skill: pin them, run your tests, and update the plan.")
		}
	}
	for _, w := range tc.WikiChanged {
		items := make([]string, len(w.Docs))
		for i, d := range w.Docs {
			if d.Removed {
				items[i] = d.ID + " (removed)"
			} else {
				items[i] = fmt.Sprintf("%s (%s)", d.ID, d.Kind)
			}
		}
		where := "The wiki of " + w.Wiki
		if !strings.HasPrefix(w.Wiki, "#") && w.Wiki == self.Name {
			where = "Your wiki"
		}
		notes = append(notes, fmt.Sprintf("%s changed: %s. Read the documents that concern your work.", where, capped(items, maxNoticeFeatures)))
	}
	for _, hub := range tc.VocabularyChanged {
		note := fmt.Sprintf("The vocabulary of #%s changed. Call get_vocabulary to read it.", hub)
		if self.System != nil {
			note += " Rename entries in your map and feature documents that use other names for its terms or interfaces."
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

// e2ePlan is the plan's file name in a tester's scope.
const e2ePlan = "plan.md"

// testerPrompt tells a tester what it tests and how (rule SK2 of the tester
// spec).
func testerPrompt(self api.Self) string {
	if self.Tests == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n## You are the tester of #%s\n\nYou prove the stories and rules of the wiki of #%s against the released products of its systems. You never see their code; you test what they release. Load the hikari-tester skill before you plan, pin, test, or report.\n\n", self.Tests, self.Tests)
	fmt.Fprintf(&b, "- Your architecture plan is %s/%s: the components of the test system, the actors that drive it, the scenarios, and the gaps you cannot test. Keep it true; the user reviews it and approves each gap.\n", self.System.WorkDir, e2ePlan)
	b.WriteString("- Pin the release set you test with mcp__hikari__pin_releases. A new pin makes every result pending: run your tests again after it.\n")
	b.WriteString("- In tests.yaml, link each test to the stories and rules it checks, as [[id]]. A story's health comes from those tests, as a feature's comes from unit tests.\n")
	fmt.Fprintf(&b, "- When a story fails, tell the orchestrator of #%s in the hub: the story, the test, the step that failed, what you expected, and what happened. Describe behavior, not code.\n", self.Tests)
	b.WriteString("- mcp__hikari__get_test_plan shows the plan as hikari reads it: its findings, the gaps with the user's approvals, and the E2E health of each story.\n")
	return b.String()
}

// wikisPrompt names the hub wikis that the agent reads (rules A1, E2, and
// S3 of the wikis spec).
func wikisPrompt(self api.Self) string {
	if len(self.HubWikis) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## The wikis of your hubs\n\nEach hub has a wiki: its stories, which say what the hub's systems must do together and for whom, its rules, which must always hold, and its principles, which guide how the systems work together. You read these, read-only:\n\n")
	for _, hub := range self.HubWikis {
		fmt.Fprintf(&b, "- #%s: %s\n", hub, api.HubWikiPath(hub))
	}
	b.WriteString("\nA document links others as [[id]]; an id is unique in every wiki, and mcp__hikari__get_document reads any document you can read, with the documents that link it. Plans and notes in a wiki are the user's: read them for context, and act on a plan only when the user asks you to.")
	if self.System != nil {
		b.WriteString(" The rules and principles of your hubs apply to your system. When a story needs a feature your system lacks, or a feature cannot keep a hub rule, tell your orchestrator.")
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
		fmt.Fprintf(&b, "- %s: %s\n", sys, api.ReleasesPath(sys))
	}
	b.WriteString("\nEach release is releases/<deliverable>/<version>/, with release.yaml (what it is, who published it, and when), SHA256SUMS, product/ (the files), and map.yaml (the system's public map at release time). releases/<deliverable>/latest is the newest release of that deliverable. Read the product's docs before you integrate a release. Choose a version, and do not edit the release; copy or install it where your work needs it.")
	if self.System != nil {
		b.WriteString(" When your tests use a release, name its system and version in the description of the suite in tests.yaml that runs them.")
	}
	b.WriteString("\n")
	return b.String()
}

// deliverablesPrompt tells an owner which deliverables it holds and how to
// publish one (rule P12).
func deliverablesPrompt(self api.Self) string {
	var b strings.Builder
	b.WriteString("\n## Your deliverables\n\n")
	if len(self.Deliverables) == 0 {
		b.WriteString("You hold no deliverable, so you cannot publish a release. When a release of your work is needed, ask the user to grant you a deliverable.\n")
		return b.String()
	}
	b.WriteString("A deliverable is a named package that says what a product must contain, where it must also be published, and how you make it. You hold these:\n\n")
	for _, d := range self.Deliverables {
		fmt.Fprintf(&b, "- %s: %s Its definition is in %s/deliverables/%s/, and you build its product in %s/staging/%s/.\n", d.Name, d.Description, api.SharedDir, d.Name, api.ExportDir, d.Name)
	}
	b.WriteString(`
To publish a release of a deliverable: load the skill with the deliverable's name and follow it to build the product in the staging directory of that deliverable, with nothing else in it. Call mcp__hikari__check_release to check the product against the deliverable's layout rules, and fix every problem it reports. Then call mcp__hikari__publish_release with the deliverable, a semver version greater than its last release, and notes that say what changed. When the deliverable has targets, publish to each target first, as its skill says, and give one certificate for each in the same call. hikari writes the manifest, moves the product into /hikari/public/export/releases/<deliverable>/<version>/, and announces the release in your hubs.
`)
	return b.String()
}

// deliverableNames returns the names of the deliverables that self holds.
func deliverableNames(self api.Self) []string {
	names := make([]string, len(self.Deliverables))
	for i, d := range self.Deliverables {
		names[i] = d.Name
	}
	return names
}

// memberDeliverables lists the deliverables of an orchestrator's members
// (rule P14).
func memberDeliverables(self api.Self) string {
	if len(self.MemberDeliverables) == 0 {
		return "- No member of your hubs holds a deliverable, so none can publish a release. You cannot change that; if a release is needed, ask the user to grant a deliverable.\n"
	}
	var b strings.Builder
	b.WriteString("- Deliverables of your members:")
	for _, m := range slices.Sorted(maps.Keys(self.MemberDeliverables)) {
		var names []string
		for _, d := range self.MemberDeliverables[m] {
			names = append(names, fmt.Sprintf("%s (%s)", d.Name, d.Description))
		}
		fmt.Fprintf(&b, " %s publishes %s;", m, strings.Join(names, ", "))
	}
	b.WriteString(" you cannot change deliverables; if a system needs another, ask the user.\n")
	return b.String()
}

// memberGrants lists the release grants of an orchestrator's members.
func memberGrants(self api.Self) string {
	if len(self.MemberGrants) == 0 {
		return "- No member of your hubs is granted another system's releases. You cannot change grants; if a system needs another system's releases, ask the user to grant them.\n"
	}
	var b strings.Builder
	b.WriteString("- Release grants of your members:")
	for _, m := range slices.Sorted(maps.Keys(self.MemberGrants)) {
		fmt.Fprintf(&b, " %s reads the releases of %s;", m, strings.Join(self.MemberGrants[m], ", "))
	}
	b.WriteString(" you cannot change grants; if a system needs others, ask the user.\n")
	return b.String()
}
