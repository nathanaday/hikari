# Spec: the Hikari plugin

Status: specified.

Sources: the conversation of 2026-10-27, which listed the gaps between today's onboarding and a plugin, and agreed the proposal that closed them. The names are those of `docs/glossary.md`.

## Summary

Hikari ships as a Claude Code plugin from this repository. Once you install it, every Claude Code session on the machine has Hikari's skills, and one of them, `hikari`, is the front door: a request such as "take these repos, build a hikari topology, and start working in hikari containers" reaches it from any folder, and it routes the request to the skill of the stage you are at. The plugin's launcher installs the hikari binary of the plugin's own version on first use, from a GitHub release whose checksums the plugin pins. The binary builds its agent image when the image is missing, checks the machine with `hikari doctor`, and lets the session post to a running project's room with `hikari send`. The host skills leave the binary and live in the plugin.

## Decisions

From the conversation:

1. The plugin is the only home of the host skills, and its version is the binary's. The binary stops writing skills into projects. One version for both answers question A2 of `docs/brainstorm/onboarding.md`, which kept the skills in the binary so that they could not drift from it.
2. The launcher installs into `~/.hikari/bin/<version>/` with no sudo. One PATH entry, written the way `install.sh` writes it today, puts hikari on the PATH of your own terminal; you run that one `sudo` line yourself.
3. The binary embeds the agent image's Dockerfile and builds the image when it is missing. Hikari publishes no image to a registry.
4. The plugin lives in `plugin/`, and the marketplace file sits at the repository root.
5. A project started from a folder of repositories goes in a new folder inside that folder.
6. Releases follow almagest's: one version, binaries for macOS and Linux on arm64 and amd64, checksums pinned into a launcher, and a workflow that tags and publishes.
7. The binary resolves symlinks when it looks for its Linux binaries, and gains `hikari doctor` and `hikari send`.
8. The plugin adds a front door and two skills: a read-only topology proposal before onboarding, and a change of topology after it.
9. A live check from a clean state, with a check that the front door's description triggers.

## Decisions made in this spec

1. **A source install takes the plugin's layout.** `install.sh` installs a build of the checkout into `~/.hikari/bin/<version>/`, points `~/.hikari/bin/hikari` at it, and writes the PATH entry for `~/.hikari/bin`. `/opt/hikari/bin` retires. Both installers write the same file, `/etc/paths.d/hikari`, so two folders would leave the PATH to whichever ran last. With one folder, the launcher runs a source build of its own version too, which is how you try a change before a release.
2. **The version lives in `plugin/.claude-plugin/plugin.json`.** The Makefile stamps it into `version.Version`; a build outside the Makefile reports `dev`. The first version is `0.1.0`.
3. **A release is triggered by its tag, not by a push to main.** Almagest checks every push to main against the pinned checksums, which works because main takes only merges from `preview`. Hikari commits to main directly, so most commits change the binary without being a release. `make version V=<x>` builds the release on your machine, pins its checksums, and commits; pushing the tag `<x>` runs the workflow, which builds again, refuses bytes that differ from the pins, attests the binaries, and publishes them.
4. **The launcher never downloads in a hook.** A hook runs at every session start with a short timeout, and the first install fetches three binaries of about 17 MB each. The download runs on the first `hikari` command of a session, which the front door makes with `hikari doctor`.
5. **The image keeps its name, `hikari-agent`, with a label.** Every topology and partition Dockerfile names `hikari-agent`, so a rebuilt image needs no edit to any of them. The label `hikari.dockerfile` holds the sha256 of the embedded Dockerfile, and `hikari up` builds the image again when the label differs. The image holds no hikari binary, since the binary is mounted at run time, so a new hikari version rebuilds the image only when the Dockerfile changed.
6. **`hikari up` writes the address of its UI to `.hikari/host.json`,** beside the topology file, readable only by you, and removes it at stop. `hikari send` reads it and posts as you, with the UI's token as its cookie. The UI's secret stays a secret of your user account, as it is today in the terminal's scrollback.
7. **`hikari sync` removes the old host skill copies.** A project made before the plugin holds `.claude/skills/containerize-repo/` and `.claude/skills/hikify/`, and a session there would see them beside the plugin's. Sync removes such a folder only when its `.gitignore` is the one hikari wrote, so a skill of your own with the same name stays.
8. **The skills run each command in the project root.** A session may start outside the project, and its working directory persists only inside its own folders. Each skill names the root, runs commands as `cd <root> && …`, and gives subagents absolute paths.
9. **A topology change keeps its record in `changes/<id>/`,** laid out as `onboarding/` is, with `<id>` the date and a short name. It reads hikify's references with that folder in place of `onboarding/`, so the two skills share one description of each stage.
10. **The front door's day-to-day route is its own skill, `room`.** Talking to running agents is the most frequent request after onboarding, and its description has to trigger on requests that name no stage.
11. **Hikify's two borrowed references are committed copies.** `make skills` copies them from the agents' skill packs into `plugin/skills/hikify/references/`, and a test fails when a copy differs from its source.
12. **The front door runs the plugin's launcher by its path once.** Claude Code puts the plugin's `bin/` on the Bash tool's PATH, but an older hikari elsewhere on the PATH could come first. The front door runs `"${CLAUDE_PLUGIN_ROOT}/bin/hikari" doctor`, which installs the right version and reports a `hikari` on the PATH of another version, so that every later `hikari` command in the skills is the plugin's.

## Requirements

### Releases and the launcher

- **CP1.** `plugin/.claude-plugin/plugin.json` holds the version. `make build` and `make release` stamp it into `version.Version` with `-ldflags -X`, and `hikari version` prints it.
- **CP2.** `make release` builds the UI, then `hikari-<version>-<os>-<arch>` for `darwin` and `linux` on `arm64` and `amd64` into `build/release/`, with `-trimpath`, `-buildvcs=false`, `CGO_ENABLED=0`, and the toolchain of `go.mod`, and writes their sha256 to `build/release/checksums.txt`.
- **CP3.** `make pin` copies the checksums to `plugin/release/checksums.txt` and writes the launcher, `plugin/bin/hikari`, from a template, with the version and the checksums inside it. `make version V=<x>` sets the version in `plugin.json` and in the marketplace entry (its `version` and its `ref`), then pins.
- **CP4.** The launcher is a POSIX shell script. It runs `~/.hikari/bin/<version>/hikari` when that file exists. Otherwise it downloads the host's asset and each Linux asset that the host's asset is not, from `https://github.com/nathanaday/hikari/releases/download/<version>/`, checks each one's sha256 against its pin, installs them as `hikari` and `hikari-linux-<arch>` in `~/.hikari/bin/<version>/`, points `~/.hikari/bin/hikari` at the new `hikari`, logs the install to `~/.hikari/install.log`, and runs it. A mismatch installs nothing. `HIKARI_HOME` moves `~/.hikari`; `HIKARI_BIN` runs that binary and downloads nothing; `HIKARI_NO_DOWNLOAD=1` never downloads; `HIKARI_RELEASE_BASE` replaces the download base, for tests.
- **CP5.** In a hook (`hikari hook …`) with no binary installed, the launcher downloads nothing and exits 0. At session start in a folder with `topology.yaml`, it prints one line that says hikari is not installed yet and that the `hikari` skill installs it.
- **CP6.** `.github/workflows/release.yml` runs on a pushed tag of the form `<major>.<minor>.<patch>`. It checks that the tag equals the plugin's version, runs the tests, runs `make release`, fails when `build/release/checksums.txt` differs from `plugin/release/checksums.txt`, attests the binaries, and publishes them with `checksums.txt` as the release of the tag. `.github/workflows/ci.yml` runs `go vet`, `go test`, and `npm test` on each push to main.
- **CP7.** `install.sh` builds the checkout, installs `bin/hikari` and `bin/hikari-linux-*` into `$HIKARI_HOME/bin/<version>/`, points `$HIKARI_HOME/bin/hikari` at it, and writes the PATH entry for `$HIKARI_HOME/bin` with sudo once: `/etc/paths.d/hikari` on macOS, `/etc/profile.d/hikari.sh` elsewhere (decision 1).

### The binary

- **CP8.** `LinuxBinary` resolves symlinks in the path of its own executable before it looks for `hikari-linux-<arch>` beside it.
- **CP9.** The agent image's Dockerfile is embedded in the binary. `hikari image` builds `hikari-agent` from it, with the label `hikari.dockerfile=<sha256 of the Dockerfile>`. Before the compose build, `hikari up` builds the image when it is missing or its label differs. `make image` runs `bin/hikari image`, and the repository's root `Dockerfile` and `.dockerignore` go.
- **CP10.** `hikari doctor [topology.yaml]` prints one line for each check, `ok` or a problem with its fix, and exits 1 when a problem blocks `hikari up`:
  - the binary, its version, and its folder;
  - a `hikari-linux-<arch>` for each Linux architecture, beside the resolved binary;
  - the PATH entry of CP7 for the folder of `~/.hikari/bin/hikari`, with the `sudo` line that writes it when it is missing (a warning, since only your terminal needs it);
  - an older install at `/opt/hikari/bin` (a warning, with the command that removes it), and a `hikari` found first on the PATH whose version differs from this binary's (a problem, since the skills would run it);
  - the Docker daemon, and `docker compose`;
  - the `hikari-agent` image and its label (a warning, since `hikari up` builds it);
  - git's `user.name` and `user.email`;
  - with a topology file, given or found in the current folder: a credential, `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`, in its `.env` or the environment. The value is never printed.
- **CP11.** `hikari up` writes `.hikari/host.json` beside the topology file, with mode 0600: the UI's URL, its token, and the process id. It removes the file at stop (decision 6).
- **CP12.** `hikari send [-to <agent>] [-file topology.yaml] <message>` posts the message as you, to the room or, with `-to`, to one agent, through the server that `.hikari/host.json` names. It prints the message's time. With no `host.json`, a dead process, or a server that does not answer, it says that hikari is not running for that topology file, and how to start it. A message of `-` is read from stdin.
- **CP13.** `hikari init` writes no skills, and its closing text says to ask Claude Code, with the plugin installed, to bring the repositories in. `hikari sync` removes the old host skill copies (decision 7), and reports each folder it removed. The UI's Create the project writes no skills.
- **CP14.** `hikari hook session-start` prints nothing outside a hikari project. In a folder whose `topology.yaml` has `x-hikari`, it prints at most a few lines: the project's name, hikari's version, the stage in `onboarding/state.md` when that file exists, the open topology changes in `changes/`, and whether hikari runs for it, from `host.json`.

### The plugin

- **CP15.** `.claude-plugin/marketplace.json` at the repository root names the marketplace `hikari`, and one plugin, `hikari`, with a `git-subdir` source: the repository `nathanaday/hikari`, the path `plugin`, and the ref of the version. `claude plugin validate` passes on the repository root and on `plugin/`.
- **CP16.** `plugin/` holds `.claude-plugin/plugin.json`, the launcher in `bin/hikari`, the pinned checksums in `release/checksums.txt`, `hooks/hooks.json` with one `SessionStart` hook that runs `"${CLAUDE_PLUGIN_ROOT}"/bin/hikari hook session-start`, and `skills/` with six skills: `hikari`, `containerize-repo`, `hikify`, `propose-topology`, `change-topology`, and `room`.
- **CP17.** The host skills move from `internal/onboard/skills/` to `plugin/skills/`. `internal/onboard` keeps the tests that the host skills name only built skills, tools, and commands; that the skills that name each other agree; and that the borrowed copies equal their sources (decision 11). The lint test of retired words reads `plugin/skills/`.

### The skills

- **CP18.** `hikari`, the front door. Its description matches requests that name hikari: bring repositories into hikari, build a hikari topology, set up or start hikari, work with hikari's agents or containers, and where a hikari project stands. It:
  1. runs the plugin's own launcher, `"${CLAUDE_PLUGIN_ROOT}/bin/hikari" doctor`, telling you first that the first run downloads hikari, and fixes each problem with you: it asks you to start Docker, gives you the `sudo` line of the PATH entry to run with `!`, runs `hikari image` with your approval, and asks for a git identity. It never reads, writes, or prints a credential: it tells you to put one in the project's `.env`, made with `claude setup-token` in your own terminal;
  2. finds where you stand: a project (a `topology.yaml` with `x-hikari` in the folder, or in a folder you name), with its stage in `onboarding/state.md` and `changes/`; or repositories, named by you or found as git repositories in the working folder and its folders, each with its `origin` and whether it is clean;
  3. routes: repositories and a request for a sketch go to `propose-topology`; repositories and a request to bring them in make the project with `hikari init <working folder>/<name>`, with a name you agree, and go to `containerize-repo`; a project goes to the skill of its stage: `containerize-repo` until `containerize: done`, then `hikify` until `hikify: done`, then `room`, or `change-topology` for a change of partitions;
  4. before the first step that costs agent time, tells you the path ahead in a few lines: containerize, design, carry out, verify, and where hikari runs.
- **CP19.** `containerize-repo` and `hikify` run in the project root (decision 8). `containerize-repo` takes repositories from local folders too: it links each by its `origin` URL, and for a repository with no `origin`, it asks whether to link its `file://` URL, for one machine, or to make a remote first. It has a section that adds a repository to a containerized project: steps 2 to 8 for that repository alone, with its baseline appended to `onboarding/baseline.md`.
- **CP20.** `hikify` reads `onboarding/proposal.md` in Stage 1 when it exists: the choices you made in it are decisions, and the rest are hypotheses that the analysis checks. In Stage 6, it posts the plan's first message with `hikari send` once you agree, instead of asking you to paste it.
- **CP21.** `propose-topology` reads repositories and writes nothing in them. It sends one read-only subagent to each repository with its own brief, all at once. From their findings it proposes partitions of folders, libraries, and flows across partitions, each boundary with a reason of hikify's three kinds, the splits it decided against, and the questions, such as a difference between two copies of code, with a `mermaid` diagram. In a project, it writes the proposal to `onboarding/proposal.md` and commits; outside one, it shows the proposal, and the front door writes it into the project that `hikari init` makes. It says that hikify checks the proposal against the agents' maps and the baseline, and names `containerize-repo` as the next step.
- **CP22.** `change-topology` changes the topology of an onboarded project: it adds a repository, splits or merges partitions, extracts a library, splits a library into its server and client parts, or moves folders between partitions. It records the change in `changes/<id>/` (decision 9), with the stages `change: scope`, `change: analysis`, `change: topology`, `change: plan`, `change: review`, `change: approved`, `change: restructure`, `change: execute`, `change: verify`, and `change: done`. It scopes the change to the partitions it touches, records their tests from `hikari status -tests` as its baseline, adds a new repository with `containerize-repo`'s section first, and follows hikify's references for each later stage, with its review rules and its approval.
- **CP23.** `room` works with a running project's agents: it checks that hikari runs, and otherwise asks you to start `hikari up` in your own terminal, since a run started by the session ends with it. It posts with `hikari send`, sending requests across partitions to the orchestration agent, and shows you each message it sends. It follows the replies in the newest `.hikari/runs/<run>/messages.jsonl`, the health with `hikari status`, and the commits with `git -C repos/<name> log hikari`. It never merges or pushes a branch without your approval.

### Docs

- **CP24.** The README's Quickstart installs the plugin, then makes the shop from a session; "Bring Your Own Repositories" starts from the front door. The onboarding guide's "Before you start", its stages, its tables, and its problems describe the plugin, `hikari doctor`, `hikari image`, and `hikari send`. `docs/design.md`'s onboarding section gives the reasons of decisions 1 to 6 of this spec. `docs/glossary.md`'s Hikari agent is a session with the Hikari plugin.

### Checks

- **CP25.** `go vet ./...`, `go test -race ./...`, `npm test`, `npm run build`, and `make build` pass. Tests cover: the launcher installs from a local release base and runs the binary, refuses a download whose sha256 differs, and in a hook downloads nothing (CP4, CP5); `LinuxBinary` through a symlink (CP8); the image's label decides a rebuild (CP9); doctor's PATH entry check (CP10); `hikari send` through `host.json` against a test server, and its message when hikari does not run (CP11, CP12); sync removes only the copies hikari wrote (CP13); the session-start text in and outside a project (CP14); and the host skill tests of CP17.
- **CP26.** A live check from a clean state: with an empty `CLAUDE_CONFIG_DIR` and an empty `HIKARI_HOME`, a headless session with only the plugin loaded installs hikari from a local release, and, given the example prompt in a folder that holds the `examples/onboard` repositories, invokes the front door, makes a project with `hikari init`, and reaches `containerize-repo`'s first question or step. A suite of `claude plugin eval` in `plugin/evals/` passes: four prompts that name hikari invoke the skill they should, and one prompt that does not name hikari invokes no Hikari skill. `hikari doctor` and `hikari image` run on this machine, and `hikari send` reaches the room of a run of the shop started with `-no-launch`.

## Build plan

1. Releases and the launcher: the version, `make release`, `make pin`, `make version`, the launcher and its tests, `install.sh`, and the workflows (CP1 to CP7).
2. The binary: symlinks, the image, doctor, `host.json`, send, the hook, and the end of written skills (CP8 to CP14).
3. The plugin: the marketplace, the manifest, the hook, and the host skills moved, with their tests (CP15 to CP17).
4. The skills: the front door, the changes to the two host skills, and the three new skills (CP18 to CP23).
5. The docs (CP24).
6. The checks (CP25, CP26).
