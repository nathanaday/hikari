---
name: hikari-health-check
description: How the orchestration agent checks the health of the project - every partition's health and staleness, the vocabulary's flags, and the health of its own features - and reports what needs work in the room, asking the partition agent that owns it. Load it when the user asks how the project stands, after a release, and when a feature of yours is failing or pending.
---

# Check the health

1. Call `list_partitions`: each partition's state and its features counted by health.
2. For each partition that is stale, or has a feature that is failing, pending, or untested, call `get_partition`: the health and state of each feature, and the flags of its interfaces.
3. Read the vocabulary's flags in `get_partition`: interface names that the vocabulary lacks, and consumed interfaces that no partition provides.
4. Read the health of your features: `search_wiki` and `get_document` show the health of each document of your feature map, computed from the partition features it links.
5. Post what needs work in the room, in one message: each failing or pending feature, with its partition; each stale partition; each flag. Ask the partition agent that owns each with @name, in terms of features and interfaces. A name that the vocabulary lacks is yours to define, or to ask a partition agent to rename.

When one of your features is failing, a feature that it links is failing in its partition's tests: ask that partition agent. When it is pending, the suites of a partition have not run since its code changed. When it is untested, no partition feature that it links has a test.

Post nothing when everything is passing and current, unless someone asked.
