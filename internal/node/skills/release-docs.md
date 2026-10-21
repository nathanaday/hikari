---
name: release-docs
description: Write the user-facing docs of the next release of your system into /hikari/public/export/staging/docs/. Run it before you publish a release.
context: fork
background: false
---

# Write the release docs

Write the user-facing docs for the next release of this system. Their reader is an engineer in another team who will install and use the release, and who cannot see this system's code.

## Inputs

Read what you need of these:

- The code in the working directory.
- `/hikari/private/layout.yaml`: where each feature lives in the code.
- `/hikari/private/test.yaml`: how each feature is tested.
- `/hikari/public/export/map.yaml`: the public map, with the interfaces the system provides.
- `/hikari/public/export/staging/release.yaml`, if it exists: the product, its runtime, and the interfaces of this release.
- `/hikari/public/export/releases/latest/docs/`, if it exists: the docs of the previous release.
- `/hikari/public/readonly/shared/policies/`: rules that every agent follows. Follow any rule about docs.

## Output

Write the docs into `/hikari/public/export/staging/docs/`, with `README.md` as the index. Add more pages when one page would be too long, and link them from the index. Change no file outside that directory.

The docs must cover:

1. What the product is and what it is for, in two or three sentences.
2. How to install and run it. Match the runtime in `release.yaml`.
3. Each interface that the release provides: what it does, its inputs and outputs, its errors, and a short example.
4. Its configuration, if there is any.
5. Its limits and known problems, from the map's findings.
6. What changed in this version, compared with the previous release.

Write short sentences in plain words. Use the names of the hub vocabulary in `/hikari/public/readonly/shared/vocabulary/`. Do not mention paths inside this system's repo, unless they are part of the product.

When you finish, reply with the list of files that you wrote.
