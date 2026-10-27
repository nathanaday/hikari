# The Vault of the Shop

This guide follows every file of the shop example, `examples/shop/topology.yaml`, from its place on the host to the place where each agent sees it. The shop has three agents: the partition agents `orders` and `inventory`, which own `services/orders` and `services/inventory` of one repository, and the orchestration agent `orchestrator`. Both partition agents hold the deliverable `python-library` and are granted the skill `python-style`, and `orders` is granted the releases of `inventory`.

Run `make demo-repo` to make the shop's repository in `.hikari/demo/shop` and its vault in `examples/shop/vault` from `examples/shop/seed`. `hikari up` makes the other directories of the vault when it starts. In the UI, each agent's inspector in the Structure tab shows its mounts as a table, from the same function that writes the override file.

## The Layout

The directory a file is in decides which containers mount it, and whether they can change it. hikari mounts a few directories of the vault in each container, one mount for each, and nothing else of the vault:

| Container path | Partition agent | Orchestration agent | Mode |
|---|---|---|---|
| `/hikari/public` | `public/` | `public/` | read-only |
| `/hikari/self` | `partitions/<id>/read-only/` | `orchestration/read-only/` | read-only |
| `/hikari/can-edit` | `partitions/<id>/can-edit/` | `orchestration/can-edit/` | writable |
| `/hikari/staging` | `partitions/<id>/staging/` | none | writable |
| `/hikari/private` | its private files | none | writable |
| `/hikari/partitions/<name>` | none | each partition's `read-only/` | read-only |
| `/hikari/grants/<name>/releases` | each granted partition's `read-only/releases/` | none | read-only |
| `/hikari/skills/<name>` | each granted skill | each granted skill | read-only |

A partition's directory in the vault is named by its partition agent's id, and its container path by the agent's name. In the shop they are the same.

## Every File

Host paths start at `examples/shop/`, except the code and the private files, which are in the repository at `.hikari/demo/shop/`. A cell says where the agent sees the file, or "none" when its container does not mount it.

### The Public Area and Your Skills

| Host path | `orders` | `inventory` | `orchestrator` |
|---|---|---|---|
| `vault/README.md` | none | none | none |
| `vault/public/vocabulary/terms.yaml` | `/hikari/public/vocabulary/terms.yaml` | `/hikari/public/vocabulary/terms.yaml` | `/hikari/public/vocabulary/terms.yaml` |
| `vault/public/wiki/<id>.md` | `/hikari/public/wiki/<id>.md` | `/hikari/public/wiki/<id>.md` | `/hikari/public/wiki/<id>.md` |
| `vault/public/policies/writing.md` | `/hikari/public/policies/writing.md` | `/hikari/public/policies/writing.md` | `/hikari/public/policies/writing.md` |
| `vault/public/deliverables/python-library/deliverable.yaml` | `/hikari/public/deliverables/python-library/deliverable.yaml` | `/hikari/public/deliverables/python-library/deliverable.yaml` | `/hikari/public/deliverables/python-library/deliverable.yaml` |
| `vault/public/deliverables/python-library/SKILL.md` | `/hikari/public/deliverables/python-library/SKILL.md` | `/hikari/public/deliverables/python-library/SKILL.md` | `/hikari/public/deliverables/python-library/SKILL.md` |
| `vault/skills/python-style/SKILL.md` | `/hikari/skills/python-style/SKILL.md` | `/hikari/skills/python-style/SKILL.md` | none |

The orchestration agent writes the vocabulary and the stories, rules, and principles of the public wiki, through its tools. The policies, the deliverables, and the skills are yours: hikari never writes them. The driver puts each policy into every system prompt, and links the skill of each held deliverable and each granted skill into `~/.claude/skills/`, where Claude Code loads them.

### The Partition `orders`

| Host path | `orders` | `inventory` | `orchestrator` |
|---|---|---|---|
| `vault/partitions/orders/read-only/features/map.yaml` | `/hikari/self/features/map.yaml` | none | `/hikari/partitions/orders/features/map.yaml` |
| `vault/partitions/orders/read-only/features/<id>.md` | `/hikari/self/features/<id>.md` | none | `/hikari/partitions/orders/features/<id>.md` |
| `vault/partitions/orders/read-only/releases/python-library/<version>/` | `/hikari/self/releases/python-library/<version>/` | none | `/hikari/partitions/orders/releases/python-library/<version>/` |
| `vault/partitions/orders/read-only/granted/<file>` | `/hikari/self/granted/<file>` | none | `/hikari/partitions/orders/granted/<file>` |
| `vault/partitions/orders/can-edit/<file>` | `/hikari/can-edit/<file>` (writable) | none | none |
| `vault/partitions/orders/staging/python-library/` | `/hikari/staging/python-library/` (writable) | none | none |
| `.hikari/demo/shop/services/orders/` | `/workspace/services/orders/` (writable) | none | none |
| `.hikari/demo/shop/services/orders/.hikari/private/code-map.yaml` | `/hikari/private/code-map.yaml` (writable) | none | none |
| `.hikari/demo/shop/services/orders/.hikari/private/tests.yaml` | `/hikari/private/tests.yaml` (writable) | none | none |
| `.hikari/demo/shop/services/orders/.hikari/private/results.yaml` | `/hikari/private/results.yaml` (hikari writes it) | none | none |

`orders` changes its feature map only through `change_wiki` and `set_partition_info`, so the map stays read-only in its container. Inside its scope, `/workspace/services/orders/.hikari` is an empty directory, so the private files appear at `/hikari/private` only.

### The Partition `inventory`

| Host path | `orders` | `inventory` | `orchestrator` |
|---|---|---|---|
| `vault/partitions/inventory/read-only/features/map.yaml` | none | `/hikari/self/features/map.yaml` | `/hikari/partitions/inventory/features/map.yaml` |
| `vault/partitions/inventory/read-only/features/<id>.md` | none | `/hikari/self/features/<id>.md` | `/hikari/partitions/inventory/features/<id>.md` |
| `vault/partitions/inventory/read-only/releases/python-library/<version>/` | `/hikari/grants/inventory/releases/python-library/<version>/` | `/hikari/self/releases/python-library/<version>/` | `/hikari/partitions/inventory/releases/python-library/<version>/` |
| `vault/partitions/inventory/read-only/granted/<file>` | none | `/hikari/self/granted/<file>` | `/hikari/partitions/inventory/granted/<file>` |
| `vault/partitions/inventory/can-edit/<file>` | none | `/hikari/can-edit/<file>` (writable) | none |
| `vault/partitions/inventory/staging/python-library/` | none | `/hikari/staging/python-library/` (writable) | none |
| `.hikari/demo/shop/services/inventory/` | none | `/workspace/services/inventory/` (writable) | none |
| `.hikari/demo/shop/services/inventory/.hikari/private/` | none | `/hikari/private/` (writable) | none |

`orders` reads the releases of `inventory`, because the topology grants them, and nothing else of `inventory`. A release holds `release.yaml`, `SHA256SUMS`, the product in `product/`, and the feature map at release time in `features/`, and `latest` points at the newest version.

### The Orchestration Agent

| Host path | `orders` | `inventory` | `orchestrator` |
|---|---|---|---|
| `vault/orchestration/read-only/features/` | none | none | `/hikari/self/features/` |
| `vault/orchestration/read-only/granted/<file>` | none | none | `/hikari/self/granted/<file>` |
| `vault/orchestration/can-edit/<file>` | none | none | `/hikari/can-edit/<file>` (writable) |

`orchestration/` exists because the topology names an orchestration agent. Its `features/` will hold the orchestration feature map, which a later step of the shared language builds.

## Files You Give an Agent

Put a file in a partition's `read-only/granted/` to give its partition agent, and the orchestration agent, a file to read, such as a specification. Put it in the partition's `can-edit/` to give the partition agent a file that it can change, such as a list of open questions. The orchestration agent has the same two places in `orchestration/`. hikari writes into neither.

In a project repository, hikari commits the partition agent's changes in its `can-edit/` with each apply, with the agent as author, and the orchestration agent's at the end of each of its turns. Your own changes in a `can-edit/` or a `granted/` are committed with your identity when hikari starts and stops. The shop example is not a project repository, so nothing in its vault is committed.

## An Old Layout

A vault from before this layout holds `shared/` and `systems/`, and the topology's directory holds `store/` in place of `vault/`; a partition's private directory holds `layout.yaml` in place of `code-map.yaml`. hikari moves none of these. `hikari check` and `hikari up` refuse each one and name the move that fixes it, and `docs/guides/migrating.md` lists every move.
