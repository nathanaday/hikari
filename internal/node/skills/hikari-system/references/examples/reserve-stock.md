---
id: reserve-stock
name: Reserve stock
description: Hold a quantity of a SKU for an order when enough is available.
interfaces: [Stock.reserve]
updated: 2026-10-22T05:57:04Z
described_commit: c69dd499311b21c5210c0ae3e984060aa0ba051a
described_at: 2026-10-22T05:57:09Z
---

## Purpose

An order must not promise stock that another order already holds. A reservation moves a quantity from available to held, so that two orders cannot both take the last unit. It refuses a request for more than is available, and it refuses a quantity that is not positive, because a reservation of nothing hides a bug in the caller. A held quantity returns to available through [[release-stock]], and [[check-availability]] reports what is left.
