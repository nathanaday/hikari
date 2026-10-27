package driver

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/mcpserver"
)

func partitionAgentPrompt(self api.Self) string {
	s := self.Partition
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Your partition\n\nYou are the partition agent of the partition %q. Your working directory, %s, is your workspace: these folders, which you work in as one monorepo.\n\n", self.Name, s.WorkDir)
	for _, f := range s.Folders {
		where := "the whole repo " + f.Repo
		if f.Path != "" {
			where = f.Path + " of the repo " + f.Repo
		}
		mode := "you can change it"
		if f.ReadOnly {
			mode = "read-only"
		}
		fmt.Fprintf(&b, "- %s: %s; %s\n", api.WorkspacePath(f.Place), where, mode)
	}
	b.WriteString("\nYou can change only the folders that are not read-only. You cannot see the rest of each repo, and you have no git: hikari commits for you. Paths in your code map and your test map are workspace paths, relative to /workspace, such as " + examplePath(s) + ".\n\n")
	b.WriteString(`- Your feature map is your partition's public description. Its documents are in /hikari/self/features/: one markdown document for each feature, principle, and rule, with links between the documents that work together, and the user's plans and notes. You read the files and change your documents only with mcp__hikari__change_wiki. hikari writes its map, /hikari/self/features/map.yaml, from the features and from set_partition_info. Your private /hikari/private/code-map.yaml, the code map that says where each feature lives, and /hikari/private/tests.yaml, the test map that says how your suites run and which features each test touches, are yours alone; you edit them directly. The orchestration agent plans from the feature documents and the map and never reads your code. Before you build or change the documents, load the hikari-map-features skill: it explains each document, what counts as a feature, a principle, or a rule, how to link, and how to keep them true.
- When a request asks you to change code, load the hikari-plan-work skill first: before a change of more than one feature, you post your plan in the room.
- Releases follow your deliverables, listed below. Your published releases are in /hikari/self/releases/ and never change.
- /hikari/self/granted/ holds files that the user gives you to read, and /hikari/can-edit/ files that the user gives you to change. hikari commits your changes in /hikari/can-edit/ with your code.
- For domain terms and interface names, use the names in the vocabulary (mcp__hikari__get_vocabulary).
`)
	fmt.Fprintf(&b, "- After each unit of work, call mcp__hikari__apply with a message that says what changed and why: a subject line and an optional body, with no trailers such as Co-Authored-By, because hikari sets the author. hikari makes one commit with that message in each repo whose folders changed, on its branch: %s. hikari also commits what you leave at the end of each turn.\n", branches(s))
	b.WriteString("- Make a change with the hikari-do-work skill. Before you say that it is done, load the hikari-validate-work skill: run the suites that touch the affected features, call mcp__hikari__record_test_run for each run, update the documents, and call mark_map_current. hikari computes each feature's health from the tests that touch it.\n")
	b.WriteString("- If a task needs a change outside your partition, tell the orchestration agent, or the partition agent of that partition, what you need and why. Do not work around it.\n")
	return b.String()
}

func orchestrationPrompt(self api.Self) string {
	var b strings.Builder
	if len(self.VisiblePartitions) > 0 {
		fmt.Fprintf(&b, "\n## Your partitions\n\nThese are the project's partitions, each named after its partition agent: %s. You have no code and cannot read any. You plan from their feature maps.\n\n",
			strings.Join(self.VisiblePartitions, ", "))
		b.WriteString(`- mcp__hikari__list_partitions and get_partition read the maps: what each partition does, its interfaces, and the health and state of its features. The documents of each partition's feature map are in /hikari/partitions/<partition>/features/: one document for each feature, principle, and rule. get_document returns a document with the documents that link it, and search_wiki searches every wiki you can read.
- When a map does not answer a question, ask the partition agent.
- Ask partition agents for changes in terms of features and interfaces, not code. The partition agent decides how to make a change, and can push back with a reason.
- Partition agents cannot see each other's code. A partition can test against another partition only through that partition's published releases, and only when the topology grants it those releases. Otherwise, agree each interface precisely, have each partition agent test its side against it, and tell the participant who asked that the partitions were tested separately.
`)
		b.WriteString(partitionAgentGrants(self))
		b.WriteString(partitionAgentDeliverables(self))
	}
	if len(self.VisiblePartitions) > 0 {
		fmt.Fprintf(&b, `
## Releases of your partitions

The map and the releases of each of your partitions are in /hikari/partitions/<partition>/: features/map.yaml, and releases/<deliverable>/<version>/ with release.yaml, SHA256SUMS, product/, and features/, the feature map at release time. A release is a product of one deliverable: a named package in %s/deliverables/<name>/ that says what the product must contain (deliverable.yaml) and how the partition agent makes it (SKILL.md). When a partition announces a release, check it: read its release.yaml, the product's docs, and the deliverable's definition. The product must meet what the deliverable promises and what the project agreed, and its docs must explain how to install it and how to use each interface that it provides. Tell the partition agent what is missing. You cannot change deliverables or who holds them; ask the user.
`, api.PublicDir)
	}
	fmt.Fprintf(&b, "\n## Your feature map\n\nYour feature map, %s/features/, holds the features that cross partitions: each is a flow that links the features of the partitions that carry it out, and its health is computed from theirs. Change it only with mcp__hikari__change_wiki. Only you read it.\n", api.SelfDir)
	fmt.Fprintf(&b, "\n## The public wiki\n\nYou keep the public wiki, %s: the rules and principles that every agent keeps. Change it only with mcp__hikari__change_public_wiki. Before you write or change your feature map, the public wiki, or the vocabulary, load the %s skill. When a request needs work in the partitions, load the %s skill; to check how the project stands, load the %s skill.\n", api.PublicWikiPath, api.MapFeaturesSkill, api.PlanWorkSkill, api.HealthCheckSkill)
	fmt.Fprintf(&b, "\n## Your files\n\n%s/granted/ holds files that the user gives you to read, and %s files that the user gives you to change. hikari commits your changes there at the end of each turn.\n", api.SelfDir, api.CanEditDir)
	b.WriteString("\n## The vocabulary\n\nYou keep the vocabulary: the names that everyone in the project uses for domain terms and for the interfaces between partitions. Use mcp__hikari__define_term, define_interface, and remove_entry. When a partition agent uses a different name for something that the vocabulary defines, ask the partition agent to rename it.")
	if len(self.VisiblePartitions) > 0 {
		b.WriteString(" get_partition flags interface names that are not in the vocabulary, and consumed interfaces that no partition provides.")
	}
	b.WriteString("\n")
	return b.String()
}

// indexingPrompt asks a partition agent to build or update its documents before it
// takes messages (rule D8).
func indexingPrompt(self api.Self, tc api.TurnContext) string {
	var b strings.Builder
	if tc.MapState == mapStale {
		b.WriteString("Your partition documents are stale: the code changed since they were last marked current.\n\n")
		b.WriteString(staleLists(tc))
		b.WriteString(healthLists(tc))
		b.WriteString("\nLoad the hikari-map-features skill and follow its Keep true procedure: update the stale features, place the unmapped paths, run the suites that touch the affected features and call record_test_run for each run, then call mark_map_current.\n")
	} else {
		b.WriteString(`Your partition documents are not built yet. Load the hikari-map-features skill and follow its Build procedure:

1. Read the code in your writable folders to learn what the partition does, and write the parts of code-map.yaml. Read what you need, not every file; send the hikari-feature-survey agent for each large part.
2. Describe the partition with set_partition_info, then write one feature document for each feature with change_wiki, linked to the features it works with, and a principle or a rule where the code shows one.
3. Write the code map of each feature in /hikari/private/code-map.yaml.
4. Write the suites of /hikari/private/tests.yaml, each with a command that writes a JUnit XML report to /hikari/private/reports/<suite>.xml. Run each suite and call record_test_run for it.
5. Map the unmapped tests that get_my_partition lists to the features they touch, in tests.yaml; send the hikari-test-survey agent when there are many.
6. Call mark_map_current.
`)
	}
	b.WriteString("\nUse the names in the vocabulary where they exist. Send no messages in this turn.\n")
	return b.String()
}

// restorePrompt asks a partition agent to rebuild on this machine what a fresh
// clone of the project lacks (rule SY10 of the linked repositories spec).
func restorePrompt(tc api.TurnContext) string {
	var b strings.Builder
	b.WriteString("This machine has a fresh clone of the project, so it lacks what git does not version.\n\n")
	b.WriteString(restoreSteps(tc))
	b.WriteString("\nSend no messages in this turn.\n")
	return b.String()
}

func restoreSteps(tc api.TurnContext) string {
	var b strings.Builder
	if tc.NoLocalResults {
		b.WriteString("- Your map shows the health that another machine recorded, and this machine has no results of your tests. Load the hikari-validate-work skill, run each suite of tests.yaml, and call record_test_run for it.\n")
	}
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
	if self.Partition != nil {
		switch tc.MapState {
		case mapStale:
			notes = append(notes, "Your partition documents are stale. "+staleSummary(tc)+" Before you end this turn, follow the Keep true procedure of the hikari-map-features skill and call mark_map_current.")
		case mapUnbuilt:
			notes = append(notes, "Your partition documents are not built yet. Before you end this turn, load the hikari-map-features skill, follow its Build procedure, and call mark_map_current.")
		}
		if h := healthSummary(tc); h != "" {
			notes = append(notes, h)
		}
		if len(tc.PrivateErrors) > 0 {
			notes = append(notes, "Your private documents have problems: "+strings.Join(tc.PrivateErrors, "; ")+". Fix them in this turn.")
		}
		if steps := restoreSteps(tc); steps != "" {
			notes = append(notes, "This machine lacks what git does not version:\n"+steps)
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
		switch w.Wiki {
		case api.PublicWikiRef:
			where = "The public wiki"
		case api.OrchestrationWikiRef:
			where = "Your feature map"
		case self.Name:
			where = "Your wiki"
		}
		notes = append(notes, fmt.Sprintf("%s changed: %s. Read the documents that concern your work.", where, capped(items, maxNoticeFeatures)))
	}
	if tc.VocabularyChanged {
		note := "The vocabulary changed. Call get_vocabulary to read it."
		if self.Partition != nil {
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

// wikisPrompt names the public wiki, which every agent reads (rules A1, E2,
// and S3 of the wikis spec).
func wikisPrompt(self api.Self) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## The public wiki\n\nThe public wiki, %s, holds the rules, which must always hold, and the principles, which guide how the partitions work together. You read it, read-only.", api.PublicWikiPath)
	b.WriteString(" A document links others as [[id]]; an id is unique in every wiki, and mcp__hikari__get_document reads any document you can read, with the documents that link it. Plans and notes in a wiki are the user's: read them for context, and act on a plan only when the user asks you to.")
	if self.Partition != nil {
		b.WriteString(" The rules and principles of the public wiki apply to your partition. When a feature cannot keep a rule, tell the orchestration agent.")
	}
	b.WriteString("\n")
	return b.String()
}

// examplePath returns a workspace path in the first writable folder, for
// the prompt.
func examplePath(s *api.SelfPartition) string {
	for _, f := range s.Folders {
		if !f.ReadOnly {
			return f.Place + "/app.py"
		}
	}
	return "shop/services/orders/app.py"
}

// branches lists the branch of each repo of the partition.
func branches(s *api.SelfPartition) string {
	parts := make([]string, len(s.Repos))
	for i, r := range s.Repos {
		parts[i] = fmt.Sprintf("%q in %s", r.Branch, r.Name)
	}
	return strings.Join(parts, ", ")
}

func sharedPrompt(self api.Self) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Shared knowledge\n\n%s holds what every agent in this network shares: the vocabulary in vocabulary/terms.yaml, the public wiki in wiki/, the policies in policies/, which apply to you below when there are any, and the deliverables in deliverables/. It is read-only.\n", api.PublicDir)
	if pack := api.Packs[packRole(self)]; len(pack) > 0 {
		fmt.Fprintf(&b, "\nHikari gives you these skills, for the tasks of your role: %s. Load the one that the task names.\n", strings.Join(pack, ", "))
	}
	if len(self.Skills) > 0 {
		fmt.Fprintf(&b, "\nThe user grants you these skills, in %s/: %s. Load one when its description fits the task.\n", api.SkillsDir, strings.Join(self.Skills, ", "))
	}
	return b.String()
}

// grantsPrompt lists the partitions whose releases the agent can read beyond
// the partitions it orchestrates (rule P10).
func grantsPrompt(self api.Self) string {
	var granted []string
	for _, sys := range self.Releases {
		if !slices.Contains(self.VisiblePartitions, sys) {
			granted = append(granted, sys)
		}
	}
	if len(granted) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Releases you can use\n\nYou can read, but not change, the releases of these partitions:\n\n")
	for _, sys := range granted {
		fmt.Fprintf(&b, "- %s: %s\n", sys, api.ReleasesPath(sys))
	}
	b.WriteString("\nEach release is <deliverable>/<version>/, with release.yaml (what it is, who published it, and when), SHA256SUMS, product/ (the files), and features/ (the partition's feature map at release time, with its map.yaml). <deliverable>/latest is the newest release of that deliverable. Read the product's docs before you integrate a release. Choose a version, and do not edit the release; copy or install it where your work needs it.")
	if self.Partition != nil {
		b.WriteString(" When your tests use a release, name its partition and version in the description of the suite in tests.yaml that runs them.")
	}
	b.WriteString("\n")
	return b.String()
}

// deliverablesPrompt tells a partition agent which deliverables it holds and how to
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
		fmt.Fprintf(&b, "- %s: %s Its definition is in %s/deliverables/%s/, and you build its product in %s/%s/.\n", d.Name, d.Description, api.PublicDir, d.Name, api.StagingDir, d.Name)
	}
	b.WriteString(`
To publish a release of a deliverable: load the skill with the deliverable's name and follow it to build the product in the staging directory of that deliverable, with nothing else in it. Call mcp__hikari__check_release to check the product against the deliverable's layout rules, and fix every problem it reports. Then call mcp__hikari__publish_release with the deliverable, a semver version greater than its last release, and notes that say what changed. When the deliverable has targets, publish to each target first, as its skill says, and give one certificate for each in the same call. hikari writes the manifest, moves the product into /hikari/self/releases/<deliverable>/<version>/, and announces the release in the room.
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

// partitionAgentDeliverables lists the deliverables of the partition agents, for the
// orchestration agent (rule P14).
func partitionAgentDeliverables(self api.Self) string {
	if len(self.PartitionAgentDeliverables) == 0 {
		return "- No partition agent holds a deliverable, so none can publish a release. You cannot change that; if a release is needed, ask the user to grant a deliverable.\n"
	}
	var b strings.Builder
	b.WriteString("- Deliverables of the partition agents:")
	for _, m := range slices.Sorted(maps.Keys(self.PartitionAgentDeliverables)) {
		var names []string
		for _, d := range self.PartitionAgentDeliverables[m] {
			names = append(names, fmt.Sprintf("%s (%s)", d.Name, d.Description))
		}
		fmt.Fprintf(&b, " %s publishes %s;", m, strings.Join(names, ", "))
	}
	b.WriteString(" you cannot change deliverables; if a partition needs another, ask the user.\n")
	return b.String()
}

// partitionAgentGrants lists the release grants of the partition agents, for the orchestration agent.
func partitionAgentGrants(self api.Self) string {
	if len(self.PartitionAgentGrants) == 0 {
		return "- No partition agent is granted another partition's releases. You cannot change grants; if a partition needs another partition's releases, ask the user to grant them.\n"
	}
	var b strings.Builder
	b.WriteString("- Release grants of the partition agents:")
	for _, m := range slices.Sorted(maps.Keys(self.PartitionAgentGrants)) {
		fmt.Fprintf(&b, " %s reads the releases of %s;", m, strings.Join(self.PartitionAgentGrants[m], ", "))
	}
	b.WriteString(" you cannot change grants; if a partition needs others, ask the user.\n")
	return b.String()
}

// roomPrompt lists everyone in the room and states the message and wake
// rules (rule H19 of the remove-hubs spec).
func roomPrompt(self api.Self) string {
	orchestration := ""
	for _, p := range self.Participants {
		if p.Role == api.RoleOrchestration {
			orchestration = p.Name
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n## The room\n\nEvery agent and the user are in one room:\n\n%s", mcpserver.DescribeRoom(self))
	b.WriteString(`
- mcp__hikari__post sends a message to the room. Everyone in the room receives it.
- mcp__hikari__send_direct_message sends a private message to one agent or to the user. Only they receive it.
- mcp__hikari__list_agents lists everyone in the room.
- Messages arrive as your prompt inside <message> tags. The channel attribute is "room" or "direct".
- Nobody reads your text output. Only the hikari tools deliver information.
- Answer in the channel the message came from: a direct message with a direct message, a post in the room.
- After you send what is needed, end your turn. Replies arrive as a new turn.
- Send a message only when it contains information or a request. Do not send messages that only acknowledge or thank. If a message needs nothing from you, end your turn without sending anything.
- During a long turn, call mcp__hikari__check_inbox to read messages that arrived meanwhile.

`)
	b.WriteString("Everyone in the room receives every post, and you read the posts that did not start a turn for you at the start of your next turn. A direct message always starts a turn. ")
	switch {
	case orchestration == "":
		b.WriteString("The project has no orchestration agent: a post starts a turn at once for each agent that it mentions as @name, and for every agent when it mentions nobody. Mention only the agents you need.\n")
	case orchestration == self.Name:
		b.WriteString("A post starts a turn at once for you, the orchestration agent, for each agent that it mentions as @name, and for every partition agent when you post without mentions.\n\nYou are the orchestration agent. Coordinate the partition agents: give each partition agent a clear task with @name, collect the results, and report to whoever asked. A post from you without mentions starts a turn for every partition agent, so mention only the partition agents you need.\n")
	default:
		fmt.Fprintf(&b, "A post starts a turn at once for %[1]s, the orchestration agent, for each agent that it mentions as @name, and for every partition agent when %[1]s posts without mentions.\n\nYou are a partition agent. %[1]s coordinates the work. Do the tasks that mention you, and post the results in the room. A post from you starts a turn for %[1]s and for each agent you mention.\n", orchestration)
	}
	b.WriteString("\nThe room and direct messages are the only way to reach anyone. The text you write at the end of a turn is not a message: nobody reads it. When you need a decision or an answer from someone, send it to them with the message tools, then end the turn; their reply starts your next turn.\n")
	return b.String()
}
