---
id: cancel-order
name: Cancel an order
description: Cancel a placed order and release the stock it holds.
interfaces: [cancel_order, Stock.release]
updated: 2026-10-22T05:57:47Z
described_commit: 18fb34a61faecd80590d70a507ed4908a37f983e
described_at: 2026-10-22T05:57:49Z
---

## Purpose

A customer can change their mind after [[create-order]] placed an order, and the stock that the order holds must go back on sale. Cancelling sets the order to cancelled and releases each item's reserved quantity through the inventory. An order can be cancelled once: a second cancellation is refused, so that stock is never released twice.
