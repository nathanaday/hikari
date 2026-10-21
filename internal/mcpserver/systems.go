package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/sysmap"
	"github.com/nathanaday/hikari/internal/vocab"
)

type hubArgs struct {
	Hub string `json:"hub" jsonschema:"name of the hub, without the #"`
}

func addVocabularyReader(s *mcp.Server, c *api.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_vocabulary",
		Description: "Read the vocabulary of a hub you are in: the agreed names of domain terms and of the interfaces between systems.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a hubArgs) (*mcp.CallToolResult, any, error) {
		v, err := c.GetVocabulary(ctx, hubName(a.Hub))
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(v)
	})
}

type interfaceArg struct {
	Name        string `json:"name" jsonschema:"the interface name, as in the hub vocabulary, for example POST /orders or order.created"`
	Kind        string `json:"kind" jsonschema:"one lowercase word, for example http, grpc, event, function, file, or cli"`
	Description string `json:"description,omitempty" jsonschema:"what the interface does"`
}

type layoutArg struct {
	Path        string `json:"path" jsonschema:"a path relative to your scope"`
	Description string `json:"description" jsonschema:"what this part of the system holds"`
}

type systemInfoArgs struct {
	Summary          *string         `json:"summary,omitempty" jsonschema:"one or two sentences on what the system does"`
	Responsibilities *[]string       `json:"responsibilities,omitempty" jsonschema:"what the system is responsible for"`
	Provides         *[]interfaceArg `json:"provides,omitempty" jsonschema:"the interfaces that the system offers to others"`
	Consumes         *[]interfaceArg `json:"consumes,omitempty" jsonschema:"the interfaces of other systems that this system uses"`
	Layout           *[]layoutArg    `json:"layout,omitempty" jsonschema:"the main parts of the code"`
	TestCommand      *string         `json:"test_command,omitempty" jsonschema:"the command that runs the tests, from your scope"`
}

type featureArgs struct {
	ID          string   `json:"id" jsonschema:"a short id in lowercase letters, digits, and dashes, unique in your system"`
	Name        string   `json:"name" jsonschema:"a short name"`
	Description string   `json:"description,omitempty" jsonschema:"what the feature does for a user or another system"`
	Interfaces  []string `json:"interfaces,omitempty" jsonschema:"names of interfaces from your provides or consumes that the feature uses"`
	Code        []string `json:"code,omitempty" jsonschema:"files or directories, relative to your scope, that implement the feature"`
	TestRefs    []string `json:"test_refs,omitempty" jsonschema:"test files or directories, relative to your scope, that cover the feature"`
	Findings    []string `json:"findings,omitempty" jsonschema:"known problems"`
	Plans       []string `json:"plans,omitempty" jsonschema:"planned changes"`
	Related     []string `json:"related,omitempty" jsonschema:"ids of related features in your system"`
}

type idArgs struct {
	ID string `json:"id" jsonschema:"the feature id"`
}

type testResultArg struct {
	Feature string `json:"feature" jsonschema:"the feature id"`
	Passed  int    `json:"passed" jsonschema:"the number of passed tests of the feature"`
	Failed  int    `json:"failed" jsonschema:"the number of failed tests of the feature"`
}

type testResultsArgs struct {
	Results []testResultArg `json:"results" jsonschema:"one entry for each feature whose tests ran"`
}

type commitArgs struct {
	Message string `json:"message" jsonschema:"the commit message: one line that says what changed and why"`
}

func addOwnerTools(s *mcp.Server, c *api.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_my_system",
		Description: "Read your full system map, with the health of each feature and the state of the map.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		v, err := c.MyMap(ctx)
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(v)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_system_info",
		Description: "Replace the given fields of your system's description. Fields you leave out stay as they are.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a systemInfoArgs) (*mcp.CallToolResult, any, error) {
		p := sysmap.SystemPatch{Summary: a.Summary, Responsibilities: a.Responsibilities, TestCommand: a.TestCommand}
		if a.Provides != nil {
			p.Provides = ptr(interfaces(*a.Provides))
		}
		if a.Consumes != nil {
			p.Consumes = ptr(interfaces(*a.Consumes))
		}
		if a.Layout != nil {
			layout := make([]sysmap.LayoutEntry, len(*a.Layout))
			for i, l := range *a.Layout {
				layout[i] = sysmap.LayoutEntry{Path: l.Path, Description: l.Description}
			}
			p.Layout = &layout
		}
		if err := c.SetSystemInfo(ctx, p); err != nil {
			return nil, nil, err
		}
		return text("Saved the system info."), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "upsert_feature",
		Description: "Add a feature to your map, or replace the feature with this id. Its recorded test results stay.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a featureArgs) (*mcp.CallToolResult, any, error) {
		err := c.UpsertFeature(ctx, sysmap.FeatureInput{
			ID: a.ID, Name: a.Name, Description: a.Description, Interfaces: a.Interfaces,
			Code: a.Code, TestRefs: a.TestRefs, Findings: a.Findings, Plans: a.Plans, Related: a.Related,
		})
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Saved feature %q.", a.ID)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "remove_feature",
		Description: "Remove a feature from your map, and the references to it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a idArgs) (*mcp.CallToolResult, any, error) {
		if err := c.RemoveFeature(ctx, a.ID); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Removed feature %q.", a.ID)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "record_test_results",
		Description: "Record the results of the tests that you ran, for each feature. hikari stores the time and computes each feature's health.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a testResultsArgs) (*mcp.CallToolResult, any, error) {
		results := make([]sysmap.TestResult, len(a.Results))
		for i, r := range a.Results {
			results[i] = sysmap.TestResult{Feature: r.Feature, Passed: r.Passed, Failed: r.Failed}
		}
		if err := c.RecordTests(ctx, results); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Recorded test results for %d feature(s).", len(results))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "mark_map_current",
		Description: "Declare that your map describes your code as it is now. hikari commits your pending changes first. Call it after the map is up to date.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		commit, err := c.MarkMapCurrent(ctx)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("The map now describes commit %s.", short(commit))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "commit_changes",
		Description: "Commit the changes in your scope to the work branch. Call it after each unit of work.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a commitArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(a.Message) == "" {
			return nil, nil, fmt.Errorf("the commit message is empty")
		}
		commit, err := c.Commit(ctx, a.Message)
		if err != nil {
			return nil, nil, err
		}
		if commit == "" {
			return text("Nothing to commit: your scope has no changes."), nil, nil
		}
		return text(fmt.Sprintf("Committed %s.", short(commit))), nil, nil
	})
}

type systemArgs struct {
	Name string `json:"name" jsonschema:"the system name, which is the name of its owner"`
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"text to find in feature ids, names, descriptions, and interfaces"`
}

type featureRefArgs struct {
	System string `json:"system" jsonschema:"the system name"`
	ID     string `json:"id" jsonschema:"the feature id"`
}

type termArgs struct {
	Hub        string   `json:"hub" jsonschema:"the hub whose vocabulary to change, without the #"`
	Term       string   `json:"term" jsonschema:"the term"`
	Definition string   `json:"definition" jsonschema:"what the term means in this hub"`
	Avoid      []string `json:"avoid,omitempty" jsonschema:"other names for the same thing that agents must not use"`
}

type vocabInterfaceArgs struct {
	Hub         string `json:"hub" jsonschema:"the hub whose vocabulary to change, without the #"`
	Name        string `json:"name" jsonschema:"the canonical interface name"`
	Kind        string `json:"kind" jsonschema:"one lowercase word, for example http, grpc, event, function, file, or cli"`
	Description string `json:"description" jsonschema:"what the interface does"`
}

type removeEntryArgs struct {
	Hub  string `json:"hub" jsonschema:"the hub whose vocabulary to change, without the #"`
	Name string `json:"name" jsonschema:"the term or interface name to remove"`
}

func addOrchestratorTools(s *mcp.Server, c *api.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_systems",
		Description: "List the systems in the hubs you orchestrate, with their summary, map state, and features counted by health.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.ListSystems(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(out) == 0 {
			return text("No system is in the hubs you orchestrate."), nil, nil
		}
		return yamlResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_system",
		Description: "Read one system: its purpose, interfaces, and features with their health. It flags interfaces that are not in your vocabularies or that no system provides.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a systemArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.GetSystem(ctx, a.Name)
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_features",
		Description: "Find features of your systems by text in their id, name, description, or interfaces.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a searchArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.SearchFeatures(ctx, a.Query)
		if err != nil {
			return nil, nil, err
		}
		if len(out) == 0 {
			return text("No feature matches."), nil, nil
		}
		return yamlResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_feature",
		Description: "Read one feature of a system.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a featureRefArgs) (*mcp.CallToolResult, any, error) {
		out, err := c.GetFeature(ctx, a.System, a.ID)
		if err != nil {
			return nil, nil, err
		}
		return yamlResult(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "define_term",
		Description: "Add a term to the vocabulary of a hub you orchestrate, or replace the term with this name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a termArgs) (*mcp.CallToolResult, any, error) {
		t := vocab.Term{Term: a.Term, Definition: a.Definition, Avoid: a.Avoid}
		if err := c.ChangeVocabulary(ctx, hubName(a.Hub), api.VocabularyChange{Op: "define_term", Term: &t}); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Defined the term %q in #%s.", a.Term, hubName(a.Hub))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "define_interface",
		Description: "Add an interface name to the vocabulary of a hub you orchestrate, or replace the interface with this name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a vocabInterfaceArgs) (*mcp.CallToolResult, any, error) {
		in := vocab.Interface{Name: a.Name, Kind: a.Kind, Description: a.Description}
		if err := c.ChangeVocabulary(ctx, hubName(a.Hub), api.VocabularyChange{Op: "define_interface", Interface: &in}); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Defined the interface %q in #%s.", a.Name, hubName(a.Hub))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "remove_entry",
		Description: "Remove a term or an interface from the vocabulary of a hub you orchestrate.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a removeEntryArgs) (*mcp.CallToolResult, any, error) {
		if err := c.ChangeVocabulary(ctx, hubName(a.Hub), api.VocabularyChange{Op: "remove", Name: a.Name}); err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("Removed %q from #%s.", a.Name, hubName(a.Hub))), nil, nil
	})
}

func interfaces(args []interfaceArg) []sysmap.Interface {
	out := make([]sysmap.Interface, len(args))
	for i, a := range args {
		out[i] = sysmap.Interface{Name: a.Name, Kind: a.Kind, Description: a.Description}
	}
	return out
}

func yamlResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return text(string(data)), nil, nil
}

func hubName(h string) string { return strings.TrimPrefix(h, "#") }

func short(commit string) string {
	if len(commit) > 10 {
		return commit[:10]
	}
	return commit
}

func ptr[T any](v T) *T { return &v }
