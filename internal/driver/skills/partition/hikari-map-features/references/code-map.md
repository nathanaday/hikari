# code-map.yaml: the code map

`/hikari/private/code-map.yaml` says where things are in your code. Only you and the human operator read it. hikari reads it to tell which features a changed path touches, and to hash a feature's files when you record test results.

```yaml
version: 1
parts:                          # the main parts of the code
  - path: shop/services/orders/orders/
    description: Order intake and cancellation.
  - path: shop/services/orders/tests/
    description: Unit tests.
features:                       # the code of each feature in the wiki
  create-order:
    code: [shop/services/orders/orders/create.py]
    notes: create_order() checks items, then reserves each one.
```

- Paths are workspace paths: relative to `/workspace`, as `ls /workspace` shows them, such as `shop/services/orders/orders/create.py`. They must exist (P3).
- A path can name a read-only folder, but only changes in your writable folders make a feature stale.
- Every key under `features` is a feature id in the wiki (P2). After a rename or a remove, update the key.
- Give every feature a `code` entry. Use a directory when the feature owns it, and files otherwise. A directory covers every file below it. Without an entry hikari cannot tell when the feature's code changed, and `get_my_partition` warns.
- `parts` holds what is not a feature: packages, helpers, fixtures, build code. A changed path that belongs to no feature is reported as unmapped; put it under a feature's `code`, or make it a part.
- `notes` is optional. Use it for what you would otherwise have to find again: entry points, tricky parts.

Keep the code map current in the same unit of work as the code: new or moved code updates `code`; a new feature gets its entry with its create. Before you change a feature, read its `code` entry to find where to work.
