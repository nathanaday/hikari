package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/console"
	"github.com/nathanaday/hikari/internal/hostfile"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/vault"
)

const releaseUsage = `Usage:
  hikari release list                                          list every release: tags, product, pins
  hikari release tag <partition> <deliverable> <version> <tag>   point a tag at a version
  hikari release untag <partition> <deliverable> <tag>          remove a tag
  hikari release pull <partition> <deliverable> <version|tag>   get a product onto this machine
  hikari release pin <consumer> <partition> <deliverable> <version|tag>
                                                               pin a release for a consumer partition
  hikari release unpin <consumer> <partition> <deliverable>     remove a consumer's pin
Each takes -file topology.yaml before its arguments. With hikari running, the
change goes through it; otherwise it is written to the vault, and hikari up
commits it as yours.
`

// release is hikari release (rules TG6, RM7, PN8, and LS2 of the releases
// spec).
func release(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, releaseUsage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet("release "+args[0], flag.ExitOnError)
	file := fs.String("file", "topology.yaml", "the topology file of the project")
	fs.Usage = func() { fmt.Fprint(os.Stderr, releaseUsage) }
	fs.Parse(args[1:])
	rest := fs.Args()
	need := func(n int) {
		if len(rest) != n {
			fmt.Fprint(os.Stderr, releaseUsage)
			os.Exit(2)
		}
	}
	shown := *file
	if rel, err := filepath.Rel(".", *file); err == nil {
		shown = rel
	}
	con := console.New(os.Stdout)
	_, live := hostfile.Live(*file)
	call := func(method, path string, in, out any) error {
		return hostfile.Call(ctx, *file, shown, method, "/api/ui/"+path, in, out)
	}
	load := func() (*topology.Topology, vault.Vault, error) {
		topo, err := loadShown(ctx, con, *file)
		if err != nil {
			return nil, vault.Vault{}, err
		}
		return topo, topo.VaultOf(), nil
	}

	switch args[0] {
	case "list":
		need(0)
		var entries []api.ReleaseEntry
		if live {
			if err := call(http.MethodGet, "releases", nil, &entries); err != nil {
				return err
			}
		} else {
			topo, st, err := load()
			if err != nil {
				return err
			}
			entries = catalogOf(st, topo.PartitionAgents())
		}
		if len(entries) == 0 {
			con.Println("  No releases yet.")
			return nil
		}
		for _, line := range releaseLines(entries) {
			con.Println("  " + line)
		}
		return nil
	case "tag", "untag":
		version := ""
		if args[0] == "tag" {
			need(4)
			version = rest[2]
			rest = []string{rest[0], rest[1], rest[3]}
		} else {
			need(3)
		}
		partition, deliverable, tag := rest[0], rest[1], rest[2]
		if live {
			req := map[string]string{"partition": partition, "deliverable": deliverable, "tag": tag, "version": version}
			if err := call(http.MethodPost, "releases/tag", req, nil); err != nil {
				return err
			}
		} else {
			_, st, err := load()
			if err != nil {
				return err
			}
			if version == "" {
				err = st.RemoveTag(partition, deliverable, tag)
			} else {
				err = st.SetTag(partition, deliverable, tag, version)
			}
			if err != nil {
				return err
			}
		}
		if version == "" {
			con.Println(fmt.Sprintf("  Removed the tag %s of %s %s.", tag, partition, deliverable))
		} else {
			con.Println(fmt.Sprintf("  %s %s %s is now %s.", partition, deliverable, version, tag))
		}
		return nil
	case "pull":
		need(3)
		_, st, err := load()
		if err != nil {
			return err
		}
		return pullOne(ctx, con, st, api.ReleaseRef{Partition: rest[0], Deliverable: rest[1], Ref: rest[2]})
	case "pin":
		need(4)
		consumer := rest[0]
		req := api.PinRequest{Partition: rest[1], Deliverable: rest[2], Ref: rest[3]}
		topo, st, err := load()
		if err != nil {
			return err
		}
		if live {
			var out api.PinResponse
			if err := call(http.MethodPost, "pins", struct {
				api.PinRequest
				Consumer string `json:"consumer"`
			}{req, consumer}, &out); err != nil {
				return err
			}
		} else {
			if err := mayPin(topo, consumer, req); err != nil {
				return err
			}
			if _, err := st.SetPin(consumer, req.Partition, req.Deliverable, req.Ref); err != nil {
				return err
			}
		}
		con.Println(fmt.Sprintf("  %s pins %s %s %s.", consumer, req.Partition, req.Deliverable, req.Ref))
		return pullOne(ctx, con, st, api.ReleaseRef{Partition: req.Partition, Deliverable: req.Deliverable, Ref: req.Ref})
	case "unpin":
		need(3)
		consumer := rest[0]
		req := api.PinRequest{Partition: rest[1], Deliverable: rest[2]}
		if live {
			if err := call(http.MethodPost, "pins/remove", struct {
				api.PinRequest
				Consumer string `json:"consumer"`
			}{req, consumer}, nil); err != nil {
				return err
			}
		} else {
			_, st, err := load()
			if err != nil {
				return err
			}
			if err := st.RemovePin(consumer, req.Partition, req.Deliverable); err != nil {
				return err
			}
		}
		con.Println(fmt.Sprintf("  %s no longer pins %s %s.", consumer, req.Partition, req.Deliverable))
		return nil
	}
	fmt.Fprint(os.Stderr, releaseUsage)
	os.Exit(2)
	return nil
}

// mayPin applies the consumer's grants to a pin made while hikari is
// stopped (rule PN8).
func mayPin(topo *topology.Topology, consumer string, req api.PinRequest) error {
	a, ok := topo.Agents[consumer]
	if !ok || a.Partition == nil {
		return fmt.Errorf("%q is not a partition", consumer)
	}
	if !slices.Contains(a.Grants.Releases, req.Partition) {
		return fmt.Errorf("the topology does not grant %s the releases of %q", consumer, req.Partition)
	}
	if !a.Grants.MayPin(req.Partition, req.Ref) {
		return fmt.Errorf("%s may pin only the tags of %s that match %s", consumer, req.Partition, strings.Join(a.Grants.Pins[req.Partition], ", "))
	}
	return nil
}

// pullOne pulls a product on this machine, in this process: a pull can
// outlast a request to the server.
func pullOne(ctx context.Context, con *console.Console, st vault.Vault, ref api.ReleaseRef) error {
	version, err := st.Resolve(ref.Partition, ref.Deliverable, ref.Ref)
	if err != nil {
		return err
	}
	if exists(filepath.Join(st.ReleaseDir(ref.Partition, ref.Deliverable, version), vault.ProductDir)) {
		con.Println(fmt.Sprintf("  The product of %s %s %s is on this machine.", ref.Partition, ref.Deliverable, version))
		return nil
	}
	if err := st.Pull(ctx, ref.Partition, ref.Deliverable, version); err != nil {
		return err
	}
	con.Println(fmt.Sprintf("  Pulled the product of %s %s %s.", ref.Partition, ref.Deliverable, version))
	return nil
}

// catalogOf reads the catalog from the files, for a stopped hikari.
func catalogOf(st vault.Vault, partitions []string) []api.ReleaseEntry {
	var out []api.ReleaseEntry
	for _, p := range partitions {
		releases, _ := st.Catalog(p)
		for _, r := range releases {
			by := st.PinnedBy(partitions, p, r.Deliverable, r.Version)
			if by == nil {
				by = []string{}
			}
			out = append(out, api.ReleaseEntry{Release: r, PinnedBy: by})
		}
	}
	return out
}

// releaseLines renders the catalog, one line for each release.
func releaseLines(entries []api.ReleaseEntry) []string {
	var out []string
	for _, e := range entries {
		head := fmt.Sprintf("%s %s %s", e.Partition, e.Deliverable, e.Version)
		if len(e.Tags) > 0 {
			head += " [" + strings.Join(e.Tags, ", ") + "]"
		}
		parts := []string{e.Released.Format("2006-01-02")}
		if e.Product {
			parts = append(parts, "product present")
		} else {
			parts = append(parts, "product absent")
		}
		if e.Remote != "" {
			parts = append(parts, "remote "+e.Remote)
		}
		if len(e.PinnedBy) > 0 {
			parts = append(parts, "pinned by "+strings.Join(e.PinnedBy, ", "))
		}
		notes, _, _ := strings.Cut(strings.TrimSpace(e.Notes), "\n")
		out = append(out, fmt.Sprintf("%s  %s: %s", head, strings.Join(parts, " · "), notes))
	}
	return out
}
