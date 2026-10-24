# The onboard example

Two repositories before hikari, for the onboarding skills:

- `server`: a small HTTP service that registers accounts. Its unit tests run anywhere; `tests/browser` drives the sign-up page with Playwright, which needs a browser.
- `cli`: a command that registers an account from the terminal. It copies the server's `accounts` module, with one difference: the server lowercases an email address, and the command does not.

`make example-onboard` writes each repository as a bare remote in `.hikari/demo/onboard/remotes/`, and an empty project from `hikari init` in `.hikari/demo/onboard/project/`. Open Claude Code in the project and run the `containerize-repo` skill with the two `file://` URLs.
