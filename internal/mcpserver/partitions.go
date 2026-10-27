package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/featuremap"
	"github.com/nathanaday/hikari/internal/vault"
	"github.com/nathanaday/hikari/internal/vocab"
	"github.com/nathanaday/hikari/internal/wiki"
)

func addVocabularyReader(s *mcp.Server, c *api.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_vocabulary",
		Description: "Read the project's vocabulary: the agreed names of domain terms and of the interfaces between partitions.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		v, err := c.GetVocabulary(ctx)
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(v)
	})
}

type interfaceArg struct {
	Name        string `json:"name" jsonschema:"the interface name, as in the vocabulary, for example POST /orders or order.created"`
	Kind        string `json:"kind" jsonschema:"one lowercase word, for example http, grpc, event, function, file, or cli"`
	Description string `json:"description,omitempty" jsonschema:"what the interface does"`
}

type partitionInfoArgs struct {
	Summary          *string         `json:"summary,omitempty" jsonschema:"one or two sentences on what the partition does"`
	Responsibilities *[]string       `json:"responsibilities,omitempty" jsonschema:"what the partition is responsible for"`
	Provides         *[]interfaceArg `json:"provides,omitempty" jsonschema:"the interfaces that the partition offers to others"`
	Consumes         *[]interfaceArg `json:"consumes,omitempty" jsonschema:"the interfaces of other partitions that this partition uses"`
}

type fieldsArgs struct {
	Name        *string   `json:"name,omitempty" jsonschema:"the new name"`
	Description *string   `json:"description,omitempty" jsonschema:"the new description: one sentence, at most 200 characters"`
	Interfaces  *[]string `json:"interfaces,omitempty" jsonschema:"the new list of interfaces; an empty list clears it"`
	Related     *[]string `json:"related,omitempty" jsonschema:"the new list of neighbor ids that the body does not link; an empty list clears it"`
}

type writeArgs struct {
	Op          string      `json:"op" jsonschema:"create, modify, rename, or remove"`
	ID          string      `json:"id" jsonschema:"the document id; for a create, a new id in lowercase letters, digits, and dashes, unique in every wiki, which becomes the file name"`
	Kind        string      `json:"kind,omitempty" jsonschema:"create: the kind of the document; in a feature map feature (the default), principle, or rule; in the public wiki story, rule, or principle"`
	Name        string      `json:"name,omitempty" jsonschema:"create: two to five words, as a user would say it"`
	Description string      `json:"description,omitempty" jsonschema:"create: one sentence, at most 200 characters"`
	Interfaces  []string    `json:"interfaces,omitempty" jsonschema:"create, a feature only: names from your provides or consumes that the feature uses"`
	Related     []string    `json:"related,omitempty" jsonschema:"create: ids of neighbors that the body does not link"`
	Body        *string     `json:"body,omitempty" jsonschema:"create and modify: the markdown body, with a ## Purpose section that cites no code; never a ## Notes section, which is the user's and which hikari keeps"`
	Fields      *fieldsArgs `json:"fields,omitempty" jsonschema:"modify: the properties to change; a property left out stays as it is"`
	To          string      `json:"to,omitempty" jsonschema:"rename: the new id"`
	Redirect    string      `json:"redirect,omitempty" jsonschema:"remove: the document that the links to the removed one point at from now on; leave it out to turn those links into plain text"`
}

type changeArgs struct {
	Writes []writeArgs `json:"writes" jsonschema:"the writes, applied in order as one change; a later write can name a document that an earlier one creates"`
}

// wikiWrites turns the tool's writes into the wiki's.
func wikiWrites(in []writeArgs) []wiki.Write {
	writes := make([]wiki.Write, len(in))
	for i, w := range in {
		writes[i] = wiki.Write{
			Op: w.Op, ID: w.ID, Kind: w.Kind, Name: w.Name, Description: w.Description, Interfaces: w.Interfaces,
			Related: w.Related, Body: w.Body, To: w.To, Redirect: w.Redirect,
		}
		if w.Fields != nil {
			writes[i].Fields = &wiki.Fields{Name: w.Fields.Name, Description: w.Fields.Description, Interfaces: w.Fields.Interfaces, Related: w.Fields.Related}
		}
	}
	return writes
}

// changeResult says what a change did, for the agent.
func changeResult(res api.ChangeResult) *mcp.CallToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "Applied %d write(s): %s.", len(res.Applied), strings.Join(res.Applied, "; "))
	if len(res.Rewritten) > 0 {
		fmt.Fprintf(&b, "\nRewrote the links in %s.", strings.Join(res.Rewritten, ", "))
	}
	for _, n := range res.Notes {
		b.WriteString("\nNote: " + n)
	}
	for _, w := range res.Warnings {
		b.WriteString("\nWarning: " + w)
	}
	return text(b.String())
}

type idArgs struct {
	ID string `json:"id" jsonschema:"the feature id"`
}

type runArgs struct {
	Suite string `json:"suite" jsonschema:"the name of the suite in tests.yaml whose run wrote its report"`
}

type checkArgs struct {
	Deliverable string `json:"deliverable" jsonschema:"the name of the deliverable whose product you staged"`
}

type publishArgs struct {
	Deliverable  string              `json:"deliverable" jsonschema:"the name of the deliverable to publish"`
	Version      string              `json:"version" jsonschema:"a semver version, such as 1.2.0, greater than the last release of this deliverable"`
	Notes        string              `json:"notes" jsonschema:"what changed in this version, in one or two sentences"`
	Certificates []vault.Certificate `json:"certificates,omitempty" jsonschema:"one certificate for each target of the deliverable, after you published there; leave it out when the deliverable has no targets"`
}

type commitArgs struct {
	Message string `json:"message" jsonschema:"the commit message: one line that says what changed and why"`
}

func addPartitionAgentTools(s *mcp.Server, c *api.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_my_partition",
		Description: "Read your feature map as others see it, with the health and state of each feature, plus what only you see: the state of the map, the stale features with their changed paths, the changed paths that belong to no feature, the failing tests and the features they touch, the pending features, the tests that no key of tests.yaml covers, the keys that match no test, warnings, and problems in your private documents.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		v, err := c.MyMap(ctx)
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(v)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_partition_info",
		Description: "Replace the given fields of your partition's public description. Fields you leave out stay as they are.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a partitionInfoArgs) (*mcp.CallToolResult, any, error) {
		p := featuremap.PartitionPatch{Summary: a.Summary, Responsibilities: a.Responsibilities}
		if a.Provides != nil {
			p.Provides = ptr(interfaces(*a.Provides))
		}
		if a.Consumes != nil {
			p.Consumes = ptr(interfaces(*a.Consumes))
		}
		if err := c.SetPartitionInfo(ctx, p); err != nil {
			return nil, nil, err
		}
		return text("Saved the partition info."), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "change_wiki",
		Description: "Change your partition's wiki as one change: create, modify, rename, or remove its features, principles, and rules in /hikari/self/features/. hikari checks every write against the rules of the hikari-map-features skill and applies all of them or none. Writes in one call can link documents that the same call creates; a document can also link the stories, rules, and principles of the public wiki. A rename rewrites every link to the document in every wiki; a remove points them at the redirect or turns them into text. The user's plans and notes are not yours to change. hikari writes the map's features from the wiki.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a changeArgs) (*mcp.CallToolResult, any, error) {
		res, err := c.ChangeWiki(ctx, false, wikiWrites(a.Writes))
		if err != nil {
			return nil, nil, err
		}
		return changeResult(res), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "record_test_run",
		Description: "Record one run of a suite of your tests.yaml. Run the suite's command first: it writes a JUnit XML report to /hikari/private/reports/<suite>.xml. hikari reads the report, keeps the result of each test, deletes the report, and computes the health of each feature from the tests that touch it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a runArgs) (*mcp.CallToolResult, any, error) {
		res, err := c.RecordRun(ctx, a.Suite)
		if err != nil {
			return nil, nil, err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Recorded suite %s: %d passed, %d failed, %d skipped.", res.Suite, res.Passed, res.Failed, res.Skipped)
		if res.Unmapped > 0 {
			fmt.Fprintf(&b, " %d test(s) touch no feature yet; get_my_partition lists them under unmapped_tests, and you map them in tests.yaml.", res.Unmapped)
		}
		for _, ch := range res.Changed {
			fmt.Fprintf(&b, "\n%s: was %s, now %s", ch.Feature, ch.From, ch.To)
		}
		return text(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "mark_map_current",
		Description: "Declare that your map and private documents describe your code as it is now. hikari checks the private documents and commits your pending changes first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		commits, err := c.MarkMapCurrent(ctx)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("The map now describes %s.", shortCommits(commits))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "check_release",
		Description: "Check the product you staged in /hikari/staging/<deliverable>/ against the deliverable's layout rules, and check that your map is current and your private documents are valid. It reports every problem and changes nothing. Run it before publish_release.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a checkArgs) (*mcp.CallToolResult, any, error) {
		check, err := c.CheckRelease(ctx, a.Deliverable)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("The product of %s meets its deliverable: %d file(s), %d bytes. It is ready to publish.", a.Deliverable, check.Files, check.Bytes)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "publish_release",
		Description: "Publish the product you staged in /hikari/staging/<deliverable>/ as a release of that deliverable, which never changes. It runs the checks of check_release, checks the version and the certificates, writes the manifest, and announces the release in the room.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a publishArgs) (*mcp.CallToolResult, any, error) {
		rel, err := c.PublishRelease(ctx, api.PublishRequest{Deliverable: a.Deliverable, Version: a.Version, Notes: a.Notes, Certificates: a.Certificates})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Published %s %s %s from %s: %d file(s). It is in %s/releases/%s/%s/.", rel.Partition, rel.Deliverable, rel.Version, shortCommits(rel.SourceCommits), rel.Product.Files, api.SelfDir, rel.Deliverable, rel.Version)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "apply",
		Description: "Commit the changes in your writable folders: one commit in each repository whose folders changed, on its work branch, with this message. Call it after each unit of work.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a commitArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(a.Message) == "" {
			return nil, nil, fmt.Errorf("the commit message is empty")
		}
		commits, err := c.Commit(ctx, a.Message)
		if err != nil {
			return nil, nil, err
		}
		if len(commits) == 0 {
			return text("Nothing to commit: your folders have no changes."), nil, nil
		}
		return text(fmt.Sprintf("Committed %s.", shortCommits(commits))), nil, nil
	})
}

type partitionArgs struct {
	Name string `json:"name" jsonschema:"the partition's name, which is the name of its partition agent"`
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"text to find in the documents' ids and text"`
}

type documentArgs struct {
	ID string `json:"id" jsonschema:"the document id, as it appears in a [[link]]"`
}

// addWikiReaders adds the tools that read the wikis, for every agent
// (rules T4 and T5 of the wikis spec).
func addWikiReaders(s *mcp.Server, c *api.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_document",
		Description: "Read one document of the wikis you can read, by its id: its text, its kind, its wiki, the documents that link it, and a feature's health or the support of a story, rule, or principle of the public wiki.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a documentArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.GetDocument(ctx, a.ID)
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_wiki",
		Description: "Find documents of the wikis you can read by text in their id or text: features, principles, rules, stories, and the user's plans and notes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a searchArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.SearchWiki(ctx, a.Query)
		if err != nil {
			return nil, nil, err
		}
		if len(out) == 0 {
			return text("No document matches."), nil, nil
		}
		return yamlResult(out)
	})
}

type termArgs struct {
	Term       string   `json:"term" jsonschema:"the term"`
	Definition string   `json:"definition" jsonschema:"what the term means in this project"`
	Avoid      []string `json:"avoid,omitempty" jsonschema:"other names for the same thing that agents must not use"`
}

type vocabInterfaceArgs struct {
	Name        string `json:"name" jsonschema:"the canonical interface name"`
	Kind        string `json:"kind" jsonschema:"one lowercase word, for example http, grpc, event, function, file, or cli"`
	Description string `json:"description" jsonschema:"what the interface does"`
}

type removeEntryArgs struct {
	Name string `json:"name" jsonschema:"the term or interface name to remove"`
}

func addOrchestrationTools(s *mcp.Server, c *api.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_partitions",
		Description: "List every partition, with its summary, map state, and features counted by health.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.ListPartitions(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(out) == 0 {
			return text("The project has no partition yet."), nil, nil
		}
		return yamlResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_partition",
		Description: "Read one partition: its purpose, interfaces, and features with their health. It flags interfaces that are not in the vocabulary or that no partition provides.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a partitionArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.GetPartition(ctx, a.Name)
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "change_public_wiki",
		Description: "Change the public wiki as one change: create, modify, rename, or remove its stories, rules, and principles in /hikari/public/wiki/. hikari checks every write against the rules of the hikari-map-features skill and applies all of them or none. A document can link the documents of every feature map. A rename rewrites every link to the document in every wiki. The user's plans and notes are not yours to change.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a changeArgs) (*mcp.CallToolResult, any, error) {
		res, err := c.ChangeWiki(ctx, true, wikiWrites(a.Writes))
		if err != nil {
			return nil, nil, err
		}
		return changeResult(res), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "define_term",
		Description: "Add a term to the vocabulary, or replace the term with this name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a termArgs) (*mcp.CallToolResult, any, error) {
		t := vocab.Term{Term: a.Term, Definition: a.Definition, Avoid: a.Avoid}
		if err := c.ChangeVocabulary(ctx, api.VocabularyChange{Op: "define_term", Term: &t}); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Defined the term %q.", a.Term)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "define_interface",
		Description: "Add an interface name to the vocabulary, or replace the interface with this name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a vocabInterfaceArgs) (*mcp.CallToolResult, any, error) {
		in := vocab.Interface{Name: a.Name, Kind: a.Kind, Description: a.Description}
		if err := c.ChangeVocabulary(ctx, api.VocabularyChange{Op: "define_interface", Interface: &in}); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Defined the interface %q.", a.Name)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "remove_entry",
		Description: "Remove a term or an interface from the vocabulary.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a removeEntryArgs) (*mcp.CallToolResult, any, error) {
		if err := c.ChangeVocabulary(ctx, api.VocabularyChange{Op: "remove", Name: a.Name}); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Removed %q from the vocabulary.", a.Name)), nil, nil
	})
}

func interfaces(args []interfaceArg) []featuremap.Interface {
	out := make([]featuremap.Interface, len(args))
	for i, a := range args {
		out[i] = featuremap.Interface{Name: a.Name, Kind: a.Kind, Description: a.Description}
	}
	return out
}

// yamlResult renders a result as YAML. A value that YAML cannot render is
// an error of the call, never a crash of the server.
func yamlResult(v any) (res *mcp.CallToolResult, _ any, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, fmt.Errorf("hikari could not render the result: %v", r)
		}
	}()
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return text(string(data)), nil, nil
}

func short(commit string) string {
	if len(commit) > 10 {
		return commit[:10]
	}
	return commit
}

// shortCommits writes commits of the form <repo>@<commit> with short
// commits, joined by commas.
func shortCommits(commits []string) string {
	if len(commits) == 0 {
		return "no commit"
	}
	out := make([]string, len(commits))
	for i, c := range commits {
		repo, commit, _ := strings.Cut(c, "@")
		out[i] = repo + "@" + short(commit)
	}
	return strings.Join(out, ", ")
}

func ptr[T any](v T) *T { return &v }
