# Spec: system graphs

Status: draft, for approval.

Sources: `docs/brainstorm/system-graphs.md` (round 1) and `system-graphs-answers.md`. Every question was agreed. D2: an idle agent is waiting. D5: ahead and behind against the remote is held for its own design (`docs/brainstorm/next-phase.md`).

## Summary

The UI gets a Map view: a node graph canvas with a force layout in the style of the Obsidian graph. The topology map draws each system, repository, and deliverable, and the edges between them. A click on a system moves the camera into it, and the system opens into its feature map: one node per feature, joined by the wiki's links. Lenses color the nodes: activity, tests, and git on the topology map; age and tests on the feature map. The server adds what the lenses need: whether each scope has uncommitted changes, each system's grants, the repositories and deliverables, and when each feature was created.

## Decisions

From round 1 and its answers:

1. Map is a view of its own, `#/map` and `#/map/<system>`. The hub cards stay on All activity, and the system page stays.
2. A click on a feature opens its document in a panel beside the canvas.
3. `d3-force` lays out, `d3-zoom` pans and zooms, and the UI draws on a 2D canvas itself.
4. The Obsidian look on the UI's white, neutral palette. Lens colors are the only color on the canvas.
5. Node kinds show by shape. Size grows with feature count on the topology map and with links on the feature map.
6. The layout is not saved. The same graph lands in the same shape.
7. Topology nodes: systems, repositories, and held deliverables. Edges: system to repository, deliverable to holder, and dashed edges between systems for release grants and for interfaces. No hubs and no agents without a system.
8. Activity lens: `working` and `indexing` cyan with a pulse, `idle` muted cyan, `starting` and `exited` gray outline, `error` red outline. The status dots in the sidebar use the same colors.
9. Tests lens: red if any feature fails, else yellow if any is pending, else green if any passes, else gray.
10. Git lens: magenta for a scope with uncommitted changes, white with a gray outline for a clean one. No remote counts.
11. Feature map: the age lens from a new code-owned property `created`, scaled across the system's features; the tests lens from each feature's health; a dashed outline for a `stale` or `undescribed` document under every lens.
12. One camera move opens a system into its features and folds them back. Reduced motion gets a fade.
13. Graph building and lens colors are plain functions with unit tests. The look and the transition are checked in a browser.

## Changes from the approved brainstorm

Writing exact requirements changed two details. Please approve or reject each.

1. **The sidebar dots follow the activity lens fully.** The brainstorm changed only the working color to cyan. With idle as muted cyan (D2), a dot that stayed gray when idle would mean something else on the map than in the sidebar. Every dot now uses the colors of decision 8.
2. **A click on a repository or a deliverable selects it.** The brainstorm said only what a click on a system or a feature does. A click on a repository or a deliverable lights it and its neighbors and opens a small panel: the repository's branch and its systems, or the deliverable's description, holders, and a link to the shared area.

## Terms

- **Canvas**: the component that lays out a graph, draws it, and handles pan, zoom, hover, drag, and click.
- **Graph**: the nodes and edges that a map gives the canvas.
- **Lens**: a function that gives each node a fill, an outline, and a pulse, from the state of what it stands for.
- **Topology map**: the graph of systems, repositories, and deliverables.
- **Feature map**: the graph of one system's features.
- **Neighbors**: the nodes that share an edge with a node.

## 1. Server

### The snapshot

- **N1.** Each system in the snapshot and in its events adds `dirty`: true when its scope has uncommitted changes, counting modified, deleted, and untracked files that git does not ignore. It adds `grants`: `{releases, deliverables}` from the topology.
- **N2.** The snapshot adds `repos`: for each repository of the topology, its `name`, its hikari `branch`, and the `systems` whose scope is in it, sorted. It holds no host path.
- **N3.** The snapshot adds `deliverables`: for each deliverable in the store, its `name`, `description`, `holders`, and `error` when it is invalid, as `/api/ui/store/shared` gives them today.
- **N4.** The server checks `dirty` for each scope every 2 seconds, in the loop that reloads the map files. When it changes, the server sends a `system` event.

### Feature creation time

- **W10.** A document gains a code-owned property, `created`: the time of the `create` write. A `rename` keeps it. A write that gives it is applied without it and warns, as W5 does. The header order is `id`, `name`, `description`, `interfaces`, `related`, `created`, `updated`, `described_commit`, `described_at`.
- **W11.** A document with no `created`, written before this change or by hand, keeps none. The server never guesses one.
- **Q7.** The map's feature entry adds `created` when the document has one.

## 2. Navigation

- **U17.** The view `{kind: 'map', system?}` has the hashes `#/map` and `#/map/<system>`. Back and forward move between them with the transition (U41).
- **U18.** The sidebar has a Map entry above the hubs. The system page has a link, Feature map, to `#/map/<system>`. The feature map has a link to the system page.
- **U19.** The Map view fills the main area. It has no composer.

## 3. The canvas

- **U20.** The canvas is one component, `GraphCanvas`, that takes a graph, a lens, and the selection, and calls back on hover, click, and background click. It knows nothing about systems or features.
- **U21.** The layout is `d3-force` with a link force, a many-body force, a collision force, and a weak centering force. Initial positions are deterministic, so the same graph lands in the same shape (decision 6).
- **U22.** The canvas runs the simulation and draws in its own `requestAnimationFrame` loop. The loop stops when the simulation has cooled and no animation runs, and starts again on any change. An idle map uses no CPU.
- **U23.** `d3-zoom` handles pan by drag and zoom by wheel and pinch, between 0.2× and 4×. Dragging a node moves it and pins it while it is held; the layout reheats a little.
- **U24.** The canvas draws at the device pixel ratio and resizes with its container.
- **U25.** Drawing: edges are thin straight lines in a light gray, dashed for the dashed kinds. Nodes are drawn above edges. A label sits under each node in the UI's small text. At a zoom below 0.6, only the labels of the hovered and selected nodes and their neighbors show.
- **U26.** Hover lights the node, its neighbors, and their edges, and dims the rest to 20% opacity, over 150 ms. A hover card shows the node's name, kind, and the lens detail (U36, U39).
- **U27.** A click on a node selects it. A click on the background clears the selection. The selected node has a darker outline, and the dimming of U26 stays on its neighbors until the selection clears.
- **U28.** A button fits the graph to the view. A map fits once when it first settles.
- **U29.** Keyboard: Tab moves the selection through the nodes in name order, Enter opens a system or a feature, Escape clears the selection or, on the feature map, goes back to the topology map.

Library versions, pinned in `package.json`: `d3-force` 3.0.0, `d3-zoom` 3.0.0, `d3-selection` 3.0.0, and their types.

## 4. The topology map

- **U30.** Nodes: one per system, drawn as a circle with a radius that grows with the square root of its feature count, from 6 to 18 pixels; one per repository, a rounded square of 10 pixels; one per deliverable that someone holds, a diamond of 10 pixels.
- **U31.** Edges: system to its repository; deliverable to each holder; system to each system whose releases it may read, dashed; system to each system that provides an interface it consumes, dashed. Two systems joined both ways by dashed edges get one edge.
- **U32.** A click on a system opens its feature map (U41). A click on a repository or a deliverable selects it and opens its panel (change 2).

## 5. The topology lenses

A lens colors the nodes it has something to say about. Every other node is the neutral node: a white fill with a gray outline.

- **U33.** Activity colors each system from its owner's status:

| Status | Node |
|---|---|
| `working`, `indexing` | cyan fill, with a slow pulse of its outline |
| `idle` | muted cyan fill |
| `starting`, `exited` | gray outline, white fill |
| `error` | red outline, white fill |

- **U34.** Tests colors each system: red if any feature is `failing`; else yellow if any is `pending`; else green if any is `passing`; else gray.
- **U35.** Git colors each system magenta when `dirty`, and the neutral node when clean. Each repository is magenta when any of its systems is dirty.
- **U36.** The hover card shows the lens detail: under activity, the status and its detail; under tests, the count of features for each health; under git, clean or uncommitted changes.
- **U37.** A legend in a corner of the canvas shows the colors of the current lens and what each means. A segmented control above the canvas picks the lens.

## 6. The feature map

- **U38.** Nodes: one per feature of the system, a circle with a radius from 5 to 14 pixels that grows with its number of neighbors. Edges: one per pair of neighbors from the map's `related`.
- **U39.** Age makes each feature cyan, with a brightness scaled linearly from its `created` between the oldest and the newest feature of the system. With one distinct time, every feature is at full brightness. A feature with no `created` is at the dimmest. The hover card shows the date.
- **U40.** Tests colors each feature by its health: red `failing`, yellow `pending`, green `passing`, gray `untested`. The hover card shows the counts of passed, failed, and pending tests.
- **U40a.** Under every lens, a feature whose state is `stale` or `undescribed` has a dashed outline, and the hover card names the state.
- **U40b.** A click on a feature opens a panel beside the canvas, a sheet from the bottom on a narrow screen. It shows the name, the health, the state, and the document, read from `systems/<system>/features/<id>.md` in the store and rendered as markdown. A `[[link]]` in the document selects that feature on the canvas.

## 7. The transition

- **U41.** Opening a system:
  1. The camera centers on the system and zooms to 3×, over 400 ms, with an ease-in-out curve. The other nodes and every edge fade out.
  2. The system node fades out as its features fade in at its position, over 200 ms.
  3. The feature layout starts from that position, so the features spread out, and the camera eases to fit them.
- **U42.** Going back runs the steps in reverse: the features fold into the system's position, the system node fades in, and the camera returns to the view the topology map had before.
- **U43.** Both maps are drawn by the one canvas, so no frame is empty. Opening `#/map/<system>` directly shows the feature map with no transition.
- **U44.** With `prefers-reduced-motion`, each transition is a 150 ms cross-fade.
- **U45.** The tests lens carries over between the maps. Each map remembers its last lens in the browser's local storage; a failed read gives the default lens: activity on the topology map, tests on the feature map.

## 8. Colors

- **U46.** The colors are tokens in `index.css`, named for what they mean: `--lens-active`, `--lens-waiting`, `--lens-pass`, `--lens-pending`, `--lens-fail`, `--lens-dirty`, `--lens-none`. The canvas reads them once from the document's style.
- **U47.** The status dots in the sidebar and the hub cards use the activity colors of U33 (change 1).

## 9. Out of scope

- Commits ahead of and behind the remote (`next-phase.md`).
- Hubs and orchestrators on the map.
- Saved layouts.
- Links between the feature maps of two systems.
- A dark theme.

## 10. Build plan

Each step ends with its tests passing and a commit to `main`.

1. **Created.** `created` in the feature document and the map (W10, W11, Q7), with tests.
2. **Snapshot.** `dirty` and `grants`, `repos`, `deliverables`, and the dirty check in the watch loop (N1 to N4), with tests.
3. **Route and types.** The view, the hashes, the sidebar entry, the links, the new wire types (U17 to U19).
4. **Graphs and lenses.** Plain functions that build both graphs from the snapshot and give each node its color under each lens (U30, U31, U33 to U35, U38 to U40a), with unit tests for every rule.
5. **Canvas.** `GraphCanvas` (U20 to U29), the color tokens (U46).
6. **Topology map.** The view, the lens control, the legend, the hover card, the panels (U32, U36, U37).
7. **Feature map.** The view and the document panel (U40b).
8. **Transition.** U41 to U45.
9. **Status dots.** U47.
10. **Browser check.** On the shop demo: both maps under every lens, the hover and selection, a host edit turning a system magenta, a failing test turning it red, the transition both ways recorded as a GIF, back and forward, a direct link, reduced motion, and a narrow screen.
11. **Docs.** README, `docs/design.md` (why one canvas with our own drawing, why lenses), and the pointers in the specs this one changes.
