# System maps: round 3

You agreed to everything in round 2 except D3, which grew into a new part of the design: a shared vocabulary that the hub orchestrator maintains. Workers use its names for the same objects, so the orchestrator can match systems by name. This round covers only that store. After it, I will write the full spec for your approval before I build anything.

Each question has my recommendation.

---

**V1. What the vocabulary holds.** I recommend two kinds of entries, and nothing else for now:

- **Terms:** domain objects and their meaning, for example `order`, `reservation`, `customer`. Each term has a definition and can list names to avoid ("not `purchase`").
- **Interfaces:** the canonical names of the connections between systems, for example `POST /orders` or the event `order.created`. Each has a kind and a description. The vocabulary does not record who provides or consumes an interface. The system maps already say that, and the orchestrator's view joins the two.

Decisions, conventions, and other free-form notes wait for the work order spec, where they have a natural home. Is a store of terms and interfaces what you meant by "a basic shared knowledge store"?

**V2. One vocabulary per hub.** I recommend that each hub has its own vocabulary, and the orchestrator of the hub is its only writer. This follows D2:

- In `team.yaml`, `lead` writes the `#product` vocabulary, and `eng-lead` writes the `#eng` vocabulary.
- `eng-lead` reads `#product`'s vocabulary as a member there, and copies the names that `#eng` needs into its own vocabulary.
- A hub does not inherit its parent hub's vocabulary.

The other option is one vocabulary for the whole network. That is simpler, but then someone must own it, and with several top-level hubs no single orchestrator does. Do you agree with one vocabulary per hub?

**V3. Who reads it.** I recommend that every participant of a hub can read the hub's vocabulary, workers included. This is the one shared document that workers see. Unlike system maps, it holds no details of any system, only the names that everyone agreed to use. Do you agree?

**V4. Where it is stored.** An orchestrator owns no repo, so the vocabulary cannot live in one. I recommend a file next to the topology file, `vocabulary/<hub>.yaml`. It persists across runs, and you can commit it with your topology. The server is the only writer, through the orchestrator's tools, as with system maps. Do you agree?

**V5. How agents use it.** I recommend:

- Orchestrator tools: `define_term`, `define_interface`, `remove_entry`.
- Tool for every participant: `get_vocabulary(hub)`.
- The system prompt tells workers to use the vocabulary's names in their maps.
- When a hub's vocabulary changes, the driver notes it in the next turn of each participant, so that owners can rename entries in their maps.

**V6. Checking maps against the vocabulary.** I recommend that the server never rejects a map change because of the vocabulary. The map must describe what the code does, even when it uses a name that the vocabulary lacks. Instead, the orchestrator's view flags each interface in a member's map that is not in the hub's vocabulary, and each interface that a system consumes but no system provides. The orchestrator then decides whether to add the name or ask the owner to rename it. Do you agree?

**V7. Draft format:**

```yaml
# vocabulary/product.yaml
# Written by hikari for the orchestrator of #product. Change it through the
# vocabulary tools.
version: 1
hub: product
terms:
  - term: order
    definition: A customer's request to buy items, from checkout until delivery or cancellation.
    avoid: [purchase, basket]
  - term: reservation
    definition: Stock held for an order until it is paid or cancelled.
interfaces:
  - name: POST /orders
    kind: http
    description: Creates an order from a checkout.
  - name: order.created
    kind: event
    description: Sent after an order is stored.
```

Is anything missing?

---

## What happens next

After your answers, I will write `docs/specs/system-maps.md`. It will collect every decision from the three rounds into one spec, with checkable requirements and a build plan. I will not start building until you approve it.
