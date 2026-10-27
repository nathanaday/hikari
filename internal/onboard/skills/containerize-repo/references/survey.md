# Survey brief

Send this brief to one fresh subagent for each repository, with `<name>` and its path, `repos/<name>`. The subagent only reads; it changes nothing.

---

Survey the repository at `repos/<name>` for a hikari project, which will run it in a Debian container as one agent's system. Read what you need to answer each point, not every file. Change nothing. Return the findings under these headings, with the file that shows each fact:

1. **Toolchain.** The languages, the build files, and the versions they name (for example `go.mod`, `package.json` with its `engines`, `pyproject.toml`, `CMakeLists.txt`). The system packages that the build and the tests need.
2. **Tests.** Each suite: the command that runs it, where its files are, and about how many tests it has. How each runner can write a JUnit XML report.
3. **Suites that cannot run in a container.** Each suite that needs a display, a browser, real hardware, a database or other service, network access, or credentials, with the evidence (an import, a fixture, a CI step, a doc). Say what it needs.
4. **Build products.** The folders and files that the build or the tests write (`__pycache__/`, `node_modules/`, `target/`, `dist/`, `.venv/`, coverage files), and whether `.gitignore` ignores each one.
5. **Conventions.** What the repository's `CLAUDE.md`, `CONTRIBUTING.md`, or README tells a contributor to do: formatting, structure, review rules. Leave out machine-specific setup.
6. **Run files.** Any `docker-compose.yml`, Dockerfile, devcontainer, or CI workflow, and what it starts or runs.
7. **Secrets.** Files that hold or expect credentials, such as `.env` or a key file. Name the file, never its content.
8. **Open questions.** What you could not decide from the repository.

Keep the answer under 400 words.
