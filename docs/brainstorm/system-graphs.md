# System graphs: round 1

## What I take from your proposal

Two graph views in the UI, drawn on one node graph canvas with a force layout in the style of the Obsidian graph:

- **The topology map** draws the topology file. Its nodes are systems, repositories, and deliverables. An edge joins a system to the repository its code is in, and a deliverable to the systems that use it.
- **The feature map** of one system draws its feature wiki: one node per feature, and an edge for each link.
- **Lenses** color the nodes. The topology map has three: activity (cyan to gray), tests (green, yellow, red, gray), and git (magenta for changes, white for clean, with commits ahead of and behind the remote). The feature map has two: age (newer is brighter) and tests.
- **Clicking a system** on the topology map opens its feature map with a smooth, animated transition.

Each question below has my recommendation. A short answer like "agree" or "no, because..." is enough.

## Terms

- **Canvas**: the one component that draws a graph, lays it out, and handles pan, zoom, hover, and click.
- **Graph**: the nodes and edges that a view gives the canvas.
- **Lens**: a rule that gives each node a color, and sometimes a badge, from the state of what the node stands for.
- **Topology map**: the graph of systems, repositories, and deliverables.
- **Feature map**: the graph of one system's features.

## What exists today

- The UI is React 19, Tailwind, and shadcn/ui on a white, neutral palette. It has no dark theme. Color appears only where it carries a signal: agent activity, health, and map state.
- The All activity view starts with `NetworkMap`: a grid of cards, one per hub, with its participants. It is not a graph.
- The snapshot already carries what the activity and test lenses need: each agent's status (`starting`, `idle`, `working`, `indexing`, `error`, `exited`), and each system's features with their health and state.
- The snapshot names each system's repo, scope, and branch. Deliverables and their holders come from a separate call, `/api/ui/store/shared`.
- The server knows nothing about git status beyond the paths that changed since the described commit. It knows nothing about remotes.
- A feature document records `updated`, but not when it was created.

---

## A. Where the graphs live

**A1. A new view, Map.** I recommend a sidebar entry, Map, at `#/map`, for the topology map, and `#/map/<system>` for a system's feature map. The URL changes with the transition, so the browser's back button reverses it and a link opens either map directly.

The cards of `NetworkMap` stay on All activity for now. Hubs are not nodes of the topology map (C3), so the cards still show something the map does not.

Do you agree?

**A2. Clicking a feature opens its document.** On the feature map, a click on a feature opens a panel at the side of the canvas with the feature's document, rendered as markdown, its health, and its state. The canvas stays visible and keeps its layout. Its links are clickable and move the selection on the canvas.

The alternative is to go to the system detail page, which lists the features. That leaves the canvas, which breaks the flow you described.

Do you agree?

**A3. The system detail page stays.** The feature map does not replace the list of features on the system page, which shows the private documents. The feature map gets a link to the system page, and the system page gets a link to the feature map.

Do you agree?

## B. The canvas

**B1. d3-force with our own canvas drawing.** I recommend `d3-force` for the layout, `d3-zoom` for pan and zoom, and drawing with the 2D canvas API ourselves. That is about 30 KB of library code, and every frame is ours to draw. The animated transition (F1) needs that control: nodes must move, fade, and change color under one clock.

The alternatives:

| Library | What it gives | Why not |
|---|---|---|
| `react-force-graph-2d` | A complete force graph on canvas. | It owns the draw loop and the camera. A transition between two graphs, and lens colors that animate, fight it. |
| Sigma.js with Graphology | WebGL, fast for many thousands of nodes. | We draw tens of nodes, at most a few hundred. Its layouts are not d3-force, and custom drawing is harder. |
| Cytoscape.js | Many layouts and a rich model. | Heavy, and its look is far from Obsidian's. |
| PixiJS, as Obsidian uses | WebGL drawing. | Only needed for thousands of nodes. |

Do you agree?

**B2. The look.** Obsidian's graph is small round nodes, thin straight edges, labels that fade in as you zoom, and a hover that lights a node and its neighbors and dims the rest. I recommend that look, on the UI's white background and neutral grays, not Obsidian's dark theme, because the UI has no dark theme. A node with no lens signal is a neutral gray. Lens colors are the only color on the canvas.

Do you agree?

**B3. Node size.** On the topology map, a system's radius grows with the square root of its feature count. Repositories and deliverables have one fixed size and their own shape: a rounded square for a repository and a diamond for a deliverable, so a node's kind shows without color. On the feature map, a feature's radius grows with its number of links, as Obsidian does.

Do you agree?

**B4. Interaction.** Drag to pan, scroll or pinch to zoom, drag a node to move it, hover to light it and its neighbors, click to select. The simulation stops when the layout settles and wakes when something moves, so an idle map costs nothing. A button fits the graph to the view. Each node shows its name; at a far zoom, only the selected and hovered names show.

Do you agree?

**B5. Layout is not saved.** Each time a map opens, the layout runs again from the same seed, so the same graph lands in the same shape. A node that you drag stays where you put it only until you leave the map. Saving positions would need a place to store them and a rule for new nodes; I recommend we leave it out.

Do you agree?

## C. The topology map

**C1. Nodes.** One node for each system (an owner of the topology), each repository under `x-hikari.repos`, and each deliverable that any owner holds. A deliverable in the store that no one holds is left out.

Do you agree?

**C2. Edges.** A system joins the repository its scope is in. A deliverable joins each system that holds it. I also recommend two edges between systems, drawn thinner and dashed, because they show how the systems depend on each other, which the three node kinds do not:

- **Releases**: a system joins each system whose releases the topology grants it, such as orders to inventory in the shop.
- **Interfaces**: a system that consumes an interface joins each system that provides it, as `get_system` already computes.

Do you want those two edge kinds, one of them, or neither?

**C3. No hubs or orchestrators.** Your list of nodes leaves out hubs and the agents without systems, such as a lead. I recommend we keep it that way in this phase: the topology map is about code and what it produces, and the hub cards stay on All activity. A later lens or layer can add hubs.

Do you agree?

**C4. Lenses color systems, and the git lens also colors repositories.** Under each lens, the nodes it says nothing about stay neutral gray: a deliverable under every lens, and a repository under the activity and test lenses.

Do you agree?

## D. The topology lenses

**D1. Activity.** From the owner's status:

| Status | Color |
|---|---|
| `working`, `indexing` | cyan, with a slow pulse |
| waiting (D2) | muted cyan |
| `idle` | gray |
| `starting`, `exited` | gray outline, no fill |
| `error` | red outline |

`error` is outside your cyan-to-gray palette. I recommend a red outline so that a broken agent never looks idle. The status dots in the sidebar use emerald for working today; I recommend they change to the same cyan, so that one color means one thing.

Do you agree?

**D2. What "waiting" means.** The driver has no waiting state. I see two candidates:

1. **Messages wait.** The agent is idle, and messages wait for its next turn. The server knows this.
2. **The agent waits for a reply.** The agent asked someone and stopped. The server does not know this; it would have to guess from the last message.

I recommend the first. Which did you mean?

**D3. Tests.** One color per system, from its features' health:

| Color | When |
|---|---|
| red | any feature is `failing` |
| yellow | else, any feature is `pending` |
| green | else, at least one feature is `passing` |
| gray | else: no feature is touched by a test, or the system has no features |

A system with some `passing` and some `untested` features is green. "All passing" in your words would make it yellow or gray instead. I recommend green, with the count of untested features in the hover card, because yellow already means "run the tests" and an untested feature does not need a run. Do you agree?

**D4. Git: changes in the working tree.** Magenta when the scope has uncommitted changes, including new files. hikari commits at the end of every turn, so the scope is dirty mostly while the owner works, or after you edit on the host. A clean node is white with a gray outline, because a white fill disappears on the white canvas.

The server checks `git status` for each scope every few seconds, as it checks the map files, and sends a change when one appears.

Do you agree?

**D5. Git: ahead and behind the remote.** This is a repository property, so the badge goes on the repository node: `↑3 ↓1` against the upstream of the hikari branch. Two questions:

1. **Fetching.** The counts are only as fresh as the last fetch. hikari never talks to a remote today. I recommend that hikari compares with the remote-tracking branch as it is on disk, and never fetches. The badge then says when that branch was last fetched, and you fetch yourself. The alternative is a fetch every few minutes, which needs your credentials to work without a prompt inside hikari.
2. **No upstream.** hikari creates its branch locally, so it has no upstream until you push it. I recommend that the badge compares with the upstream of the branch when it has one, and otherwise with the upstream of the branch hikari started from, such as `origin/main`, and says which.

Do you agree with both?

## E. The feature map

**E1. Nodes and edges.** One node per feature, one edge per pair of neighbors, both from the map's `related`, which already joins links in both directions. Links never cross systems, so the feature map holds one system.

Do you agree?

**E2. Age needs a creation time.** A document records `updated`, not when the feature was first written. I recommend a new property owned by code, `created`, which the server sets when it creates the document and keeps through every write, including a rename. A document without it, from before this change, has no known age.

Your words were "time of capture", which is `created`. The other choice is `updated`, which hikari has for every document, but it changes with every edit, so a feature the owner just reworded would look new. Do you want `created`, `updated`, or both as two lenses?

**E3. How age maps to brightness.** The newest feature is the brightest and the oldest the dimmest, scaled between the two across the system's features, not on a fixed clock. A system built in one turn then shows all its features at one brightness, which is true. A feature with no `created` gets the dimmest color. Do you agree?

**E4. Tests.** Each feature takes the color of its health: red `failing`, yellow `pending`, green `passing`, gray `untested`. Do you agree?

**E5. The state of a document.** A feature whose document is `stale` or `undescribed` is not a lens of yours. I recommend a thin dashed outline on such a feature under every lens, the way the feature list shows the state in amber today. Do you want it, or a third lens, or nothing?

## F. The transition

**F1. One camera move.** When you click a system:

1. The camera centers on the system and zooms in, in about 400 ms, while the other nodes fade.
2. The system node grows and fades, and its features appear at its position.
3. The feature layout starts from that point, so the features spread out from where the system was.

Going back runs the same steps in reverse: the features fold into the system node, and the camera zooms out to the topology map as it was. Both maps live on the one canvas, so no frame is blank. A user who prefers reduced motion gets a short fade instead.

Do you agree?

**F2. The lens carries over where it can.** The tests lens on the topology map opens the feature map on its tests lens. The activity and git lenses have no feature equivalent, so the feature map opens on tests. Each map remembers its last lens in the browser. Do you agree?

## G. What the server must add

**G1. New data.** For the lenses above, the snapshot and its events need:

- for each system: whether its scope has uncommitted changes (D4), and whether messages wait for its owner (D2);
- for each repository: its path, its hikari branch, and the ahead and behind counts with the ref they compare to and when it was fetched (D5);
- for each deliverable: its holders, in the snapshot, so the map needs no second call;
- for each feature: `created` (E2).

Do you agree that this phase includes these server changes?

## H. Testing

**H1. Logic out of the canvas.** Building each graph from the snapshot, and each lens's color for each node, are plain functions with unit tests. The canvas only draws what they return. The look and the transition are checked in the browser, on the shop demo, with screenshots and a recording of the transition.

Do you agree?
