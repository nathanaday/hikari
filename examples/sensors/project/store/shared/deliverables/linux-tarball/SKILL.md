---
name: linux-tarball
description: How to make a release of the linux-tarball deliverable - a static Linux binary under bin/ with a README. Load it before you publish a release of linux-tarball.
---

# A Linux program

1. Build a static binary for Linux on this machine's architecture into the staging directory:
   `CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -o /hikari/public/export/staging/linux-tarball/bin/<name> .`
2. Write `README.md` in the staging directory: what the program does, its flags and their defaults, what it reads and sends, and an example command.
3. Call `check_release`, then `publish_release`.
