# The form of onboarding/state.md

```markdown
# Onboarding

Stage: containerize: survey
Updated: 2026-10-23

## Repositories

### server

- URL: https://github.com/acme/server.git
- Does: the acme HTTP server.
- Toolchain: Python 3.11, pytest.
- Tests: tests/ (42 tests). Left out: tests/browser (needs Playwright and a browser).
- Ignore lines: added `__pycache__/` with the user's approval.
- Conventions: ruff; one handler for each module.
- Run files: docker-compose.yml starts the server with Postgres.

## Decisions

- 2026-10-23: The user named two repositories, server and cli.

## Open questions

- Does cli need network access in its tests?
```

The stages, in order: `containerize: survey`, `containerize: write`, `containerize: run`, `containerize: done`. The hikify skill adds its own stages after these.
