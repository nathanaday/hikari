---
id: check-availability
name: Check availability
description: Report how much of a SKU is free to sell.
interfaces: [Stock.available]
related: ["[[restock]]"]
updated: 2026-10-22T05:57:04Z
described_commit: c69dd499311b21c5210c0ae3e984060aa0ba051a
described_at: 2026-10-22T05:57:09Z
---

## Purpose

The shop must know what it can still sell: the stock level minus what orders hold. A SKU that the inventory has never seen is free to the amount of zero, not an error, so that a catalog can list items before the first delivery. The body links nothing, so the related property names the one neighbor that the prose does not: a restock raises what this feature reports.

## Notes

The operator asked whether negative stock should ever be reported. It is not: a release above the held quantity is refused upstream.
