# Set Up Releases That Travel

This guide is for whoever configures the releases of a hikari project with many partitions: a person, or an agent working on the topology file. It says what to write in the vault and the topology so that every partition publishes to a home outside the vault, every consumer gets what it pins, and a fresh clone of the project works. The spec is `docs/specs/releases.md`; the deliverables themselves are in `docs/specs/deliverables.md`.

## What You Are Configuring

A release is a record and a product. The record is text that hikari writes and commits with the vault: `release.yaml`, `SHA256SUMS`, and a copy of the feature map. The product is the files, which stay out of git. hikari moves products through a remote: a pair of commands that the project writes and hikari runs on the host. Tags are mutable names for versions. A pin is a consumer's declaration of the release it uses.

You write four things:

1. One remote for each kind of home, in `vault/public/remotes/<name>/remote.yaml`.
2. For each deliverable, its remote and its tags, in `vault/public/deliverables/<name>/deliverable.yaml`.
3. For each partition agent, its grants in the topology: which deliverables it holds, which tags it may set, whose releases it reads, and which tags it may pin.
4. The release skill of each deliverable, `SKILL.md`, which tells the holder how to build the product.

hikari ships no remote, no client, and no credentials. The commands are yours, and they run with the environment of the machine that runs hikari.

## 1. Remotes

A remote lives at `vault/public/remotes/<name>/remote.yaml`. The name follows the rule of deliverable names: lowercase letters, digits, and dashes, up to 32 characters.

```yaml
version: 1
description: GitHub releases of acme/schemas.
push: |
  tar -C "$HIKARI_PRODUCT" -czf "/tmp/$HIKARI_DELIVERABLE-$HIKARI_VERSION.tgz" . &&
  gh release create "$HIKARI_DELIVERABLE-$HIKARI_VERSION" --repo acme/schemas \
    --title "$HIKARI_DELIVERABLE $HIKARI_VERSION" --notes-file "$HIKARI_RELEASE/release.yaml" \
    "/tmp/$HIKARI_DELIVERABLE-$HIKARI_VERSION.tgz" "$HIKARI_RELEASE/SHA256SUMS"
pull: |
  gh release download "$HIKARI_DELIVERABLE-$HIKARI_VERSION" --repo acme/schemas \
    --pattern '*.tgz' --output - | tar -C "$HIKARI_PRODUCT" -xzf -
timeout: 10m
```

`push` and `pull` are optional, and at least one must be set. `timeout` is a duration, 10 minutes when left out. Any other key is refused.

hikari runs a command with `sh -c`, in the vault's directory, with the host's environment and these variables:

| Variable | Value |
|---|---|
| `HIKARI_REMOTE` | the remote's name |
| `HIKARI_PARTITION` | the partition that owns the release |
| `HIKARI_DELIVERABLE` | the deliverable |
| `HIKARI_VERSION` | the version, such as `1.2.0` |
| `HIKARI_DIGEST` | the product digest, `sha256:` and 64 hex characters; empty on the pull of `record_release` |
| `HIKARI_PRODUCT` | on a push, the directory to upload; on a pull, the empty directory to fill |
| `HIKARI_RELEASE` | the record's directory, which holds `release.yaml` and `SHA256SUMS` |
| `HIKARI_VAULT` | the vault's directory |

The rules a remote must keep:

- A pull fills `HIKARI_PRODUCT` with exactly the files of the release. hikari compares them with `SHA256SUMS` and rejects any difference, so a pull must not add a wrapper directory, a changelog, or metadata beside the product. Archive the product on push and extract it in place on pull, as the example does.
- A push should be safe to run twice. A publish whose push fails leaves no release, and the holder retries with the same call, so a push that refuses an existing tag or key makes the retry fail too. Prefer commands that overwrite, or that check before they create.
- A command that exits non-zero, or runs past the timeout, fails with its last lines of output in the error that the holder sees. Print what went wrong.
- The credentials are the host's: a `gh auth login`, an AWS profile, or an environment variable set where hikari runs. Containers need none for remotes. The host also needs the tools the commands name.

Write one remote for each kind of home. A library and an operating-system image do not share one, because their homes differ:

```yaml
# vault/public/remotes/edge-images/remote.yaml
version: 1
description: The S3 bucket of edge images, ${EDGE_IMAGES_BUCKET}.
push: aws s3 sync "$HIKARI_PRODUCT" "s3://${EDGE_IMAGES_BUCKET}/$HIKARI_PARTITION/$HIKARI_DELIVERABLE/$HIKARI_VERSION/" --delete
pull: aws s3 sync "s3://${EDGE_IMAGES_BUCKET}/$HIKARI_PARTITION/$HIKARI_DELIVERABLE/$HIKARI_VERSION/" "$HIKARI_PRODUCT"
timeout: 60m
```

A remote with only `pull` is for products that a CI workflow uploads, which section 4 covers. A directory on the machine is a remote too, and the shop example uses one, `examples/shop/seed/public/remotes/local-dir/remote.yaml`; it is enough for a dry run and for a team on one machine, and nothing else.

## 2. Deliverables

Each deliverable names its remote and declares its tags:

```yaml
# vault/public/deliverables/json-schemas/deliverable.yaml
version: 1
description: The JSON schemas of the ecosystem, which every SDK and service validates against.
layout:
  require: [README.md, "schemas/**/*.json"]
  forbid: ["**/.DS_Store"]
remote: github-schemas
tags: [dev, staging, production]
```

`remote` is one name of `vault/public/remotes/`; a deliverable has at most one, and a missing or invalid remote makes the deliverable invalid, which stops `hikari up`. A deliverable without `remote` is local only: its products exist on the machine that published them, a clone cannot get them, and `hikari sync` says so. Use that only for a product nothing else depends on.

`tags` is a list of patterns. Each is a regular expression anchored at both ends, and a plain word matches itself, so `[dev, staging, production]` declares three tags and `["rc-[0-9]+"]` declares a family. A tag name is lowercase, starts with a letter, and may hold digits, dots, underscores, and dashes. `latest` is reserved: hikari points it at the newest version at every publish, and nobody sets it. A deliverable without `tags` has only `latest`.

hikari gives a tag no meaning. Choose names the project reads the same way everywhere, write what each means in the deliverable's `SKILL.md` or a policy, and keep the same set across the deliverables that move through the same stages.

The holder builds the product with the deliverable's `SKILL.md`, as before. Add to that skill which tag a new release gets, and who moves the others; the shop's `python-library` skill is a short example.

## 3. Grants in the Topology

Four keys under `grants` configure a partition agent:

```yaml
services:
  p3-schemas:
    x-hikari:
      partition: {folders: [{repo: p3-schemas, path: .}]}
      grants:
        deliverables: [json-schemas]
        tags: {json-schemas: [dev, staging]}
  p3-sdk:
    x-hikari:
      partition: {folders: [{repo: p3-sdk, path: .}]}
      grants:
        deliverables: [sdk-cpp]
        tags: {sdk-cpp: [dev]}
        releases: [p3-schemas]
        pins: {p3-schemas: [staging, production]}
  edge-core:
    x-hikari:
      partition: {folders: [{repo: edge, path: core}]}
      grants:
        releases: [p3-schemas, p3-sdk]
        pins: {p3-sdk: [production]}
```

- `deliverables` lists what the agent publishes. Only an agent with a partition holds deliverables.
- `tags` restricts, for a held deliverable, the tags the agent may set, as patterns. A deliverable left out of the map may get any of its tags from this agent. A key that is not a held deliverable is refused.
- `releases` lists the partitions whose releases the agent reads, mounted at `/hikari/grants/<partition>/releases`. This is unchanged.
- `pins` restricts, for a partition in `releases`, what the agent may pin: only a tag that matches one of its patterns, and never a version. A partition left out may be pinned by any version or tag. A key that is not in `releases` is refused.

The pattern above is the usual one. A holder may set the early tags, `dev` and `staging`, and the user promotes to `production` with `hikari release tag`, since no grant allows it. A consumer that must track only promoted releases pins `production`, and one that integrates early pins `staging` too. The orchestration agent reads every release and pins nothing.

`hikari check topology.yaml` refuses an invalid grant before anything starts. Run it after each change.

## 4. Releases Built Elsewhere

When a CI workflow builds and uploads the product, the holder records the release instead of building it:

1. The remote of the deliverable has a `pull` that gets the version the workflow uploaded. `push` can be left out, which also keeps anyone from uploading from a container or a laptop.
2. The holder's skill says how the workflow is started and how to know the version it produced, for example a tag it pushes, or a run it waits for with `gh run watch`.
3. The holder calls `record_release` with the deliverable, the version, notes, and tags. hikari pulls the version into the holder's empty staging directory, checks it against the deliverable's layout rules, writes the record, and announces the release. The record names the remote, and other machines pull from it as they do for a release built in the container.

`record_release` refuses a staging directory that is not empty. The map of the partition must be current, as for any publish, so the record points at the commit the workflow built.

## 5. What Consumers Do

A consumer pins the release it uses with `pin_release`, by a version or by a tag it follows. hikari writes the pin in the consumer's `pins.yaml`, commits it as the consumer, and pulls the product onto the machine when it is absent. The consumer then finds the product at `/hikari/grants/<partition>/releases/<deliverable>/<version>/product/`, or at `.../tags/<tag>/product/` when it pins a tag.

Tell consumers, in a policy or in your request to them, to pin before they integrate, and to name the partition and version in the description of the test suite that runs against a release. A consumer that reads a release without a pin works on this machine and breaks on a clone.

On a fresh clone, `hikari sync` pulls the product of every pin after it clones the repositories, and prints one line for each. A pin whose deliverable has no remote is a warning there, and `hikari check` and `hikari up` warn about it too; they do not stop.

## 6. Promotion

Promotion is a tag move. A holder within its grant calls `tag_release`; you call `hikari release tag <partition> <deliverable> <version> <tag>`, with no grant check, and `hikari release untag` removes one. A tag change is committed and announced in the room, so a consumer that follows the tag sees it in its next turn, and its container's `tags/<tag>` link already points at the new version. No rebuild and no second upload happen, because the version did not change.

## 7. Check the Setup

Run these before you hand the project to the agents, and again from a fresh clone:

```sh
hikari check topology.yaml          # grants, deliverables, and remotes are valid
hikari release list                 # the catalog: tags, product present or absent, pins
hikari sync topology.yaml           # on a clone: pulls the products of the pins
```

For a dry run, clone the project to a temporary directory, run `hikari sync`, and read the lines for the pins. Every pinned release of a deliverable with a remote must read `pulled <version>` or `present at <version>`. Then `hikari release list` must show `product present` for each of them. A line that says `cannot be pulled` names a deliverable that still has no remote.

Each holder's first real release is the test of its push: ask the holder to publish, read the room for the announcement, and check the home with the tool of the remote.

## Reference

Tools of every agent: `list_releases`, `pull_release`. Tools of a holder: `publish_release` (with `tags`), `record_release`, `tag_release`. Tools of a consumer: `pin_release`, `unpin_release`.

Your commands: `hikari release list | tag | untag | pull | pin | unpin`, with hikari running or stopped. When it is stopped, a tag or a pin is written to the vault and committed as yours at the next `hikari up`.

Files, under `vault/`:

| Path | Who writes it | In git |
|---|---|---|
| `public/remotes/<name>/remote.yaml` | you | yes |
| `public/deliverables/<name>/deliverable.yaml`, `SKILL.md` | you | yes |
| `partitions/<id>/read-only/releases/<deliverable>/<version>/release.yaml`, `SHA256SUMS`, `features/` | hikari | yes |
| `partitions/<id>/read-only/releases/<deliverable>/<version>/product/` | hikari, through the remote | no |
| `partitions/<id>/read-only/releases/<deliverable>/tags.yaml` | hikari | yes |
| `partitions/<id>/read-only/releases/<deliverable>/tags/<tag>` | hikari, links | no |
| `partitions/<id>/read-only/pins.yaml` | hikari | yes |

Pitfalls seen so far:

- A pull that wraps the files in a directory, or adds a file, fails the checksum comparison. Fill `HIKARI_PRODUCT` with the product and nothing else.
- A holder that publishes with a tag outside its `grants.tags` is refused before anything is written, and so is a consumer that pins a version when `grants.pins` names the partition.
- A deliverable whose `remote` names a remote that does not exist is invalid, and `hikari up` refuses to start until it is fixed.
- A release published before this version of hikari has its record committed at the next start, but its product cannot be pulled unless the deliverable now has a remote and the product is at the home the remote reads. Publish it again, or upload it to the home by hand at the path the pull expects.
