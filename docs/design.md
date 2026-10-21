# Design notes

This document gives the reasons for the main decisions in hikari.

## Hubs, not a free graph

The topology is a set of hubs. Each hub has one orchestrator and zero or more
members, and the user is in every hub. Everyone in a hub can post to it, and
any two nodes that share a hub can send direct messages to each other.

A hub matches how a team works: one agent coordinates, the others do focused
work, and everyone can read the room. An agent that is a member of one hub and
the orchestrator of another connects the two, so a tree of hubs gives a
hierarchy in which each agent sees only its own rooms. Direct messages keep a
side conversation out of the room, but they never reach outside the hubs that
two nodes share.

## Not every post starts a turn

Every participant of a hub receives every post, but a post starts a turn at
once only for the orchestrator, for each participant that the post mentions as
`@name`, and for all members when the orchestrator posts without mentions.
Other participants read the post at the start of their next turn.

If every post started a turn for every member, one message in a room of N
agents would start N turns, and each reply would start N more. With these
rules, the orchestrator decides who works, a member wakes another member only
by name, and nothing is lost: every post still reaches every participant. The
server records in each message which participants it woke, and the UI shows
this under each post.

## One server routes all messages

Every message goes through the server. The server knows the topology, checks
each message against the hubs, and records each message in the log.

The other option was to connect agents directly, with Docker networks that
follow the topology. That design spreads the topology across Docker networks,
makes the log a copy of traffic, and lets an agent with a shell reach any
service on a shared network. With one server, there is one place to check
access and one log that is always complete.

## Identity comes from a token

The server gives each container a random token at start. The server finds the
sender from the token, never from the request body. An agent with a shell can
read its token and call the server with `curl`, but it can act only as itself.
That is the same reach that its MCP tools give it.

## Containers cannot reach each other

Each container has its own bridge network. Containers can reach the internet,
because Claude Code needs the Anthropic API, and the server on the host. They
cannot reach each other.

## The UI needs its own secret

Containers can reach the server, so the server cannot trust a request because
it arrives at the server address. The UI endpoints need a secret that the
server prints at start, as Jupyter does. Without the secret, an agent could
read the full log or send messages as the user.

## Agents run headless, one turn per batch of messages

The driver in each container waits for a message that wakes its agent. Then it
takes every pending message and runs `claude -p --resume <session>` with the
messages as the prompt. The agent keeps its context across turns because every
turn resumes the same session. Messages that arrive during a turn wait for the
next turn, or the agent reads them with `check_inbox`.

Two other options exist:

- An interactive session that calls a blocking `wait_for_message` tool in a
  loop. The model can stop the loop, and a long tool call can time out.
- Claude Code channels, which push MCP notifications into a live session.
  This feature is a research preview. It is a good candidate when it becomes
  stable, because it removes the per-turn process start.

The headless loop is predictable, and its output (stream-json) also gives
the driver tool calls and cost for each turn.

## The server runs on the host

The server must start containers. A server in a container needs the Docker
socket, which gives full control of the host. On the host, the server is a
plain process that is easy to run and to debug.

On Docker Desktop, containers reach a host service on 127.0.0.1 through
`host.docker.internal`. On Linux, `host.docker.internal` maps to the Docker
bridge address. There, start the server with `-listen 0.0.0.0:7420`, or listen
on the bridge address, so that containers can reach it.

## The binary embeds the UI

The UI is a Vite and React app in `web/`. The `hikari` binary embeds its build
output, so a run needs no Node.js and no second process. The `web/dist/.keep`
file lets the Go code compile before the first UI build; the server then
serves a page that asks for `make ui`. The agent image gets only that empty
directory, because containers never serve the UI.

The UI signs in by posting the token from the printed URL to the server, which
answers with a cookie. A sign-in through an API call, not a page redirect,
works the same behind the Vite dev server, which owns the page in development.

The composer shows who a draft will wake. To do this, `web/src/lib/rules.ts`
repeats the wake rules of the server. The tests of both use the same cases.

## Known limitations

- Agents can still send messages to each other without end, for example by
  mentioning each other. The wake rules and the system prompt make this less
  likely, but no hard limit exists.
- If a turn fails, the messages of that turn are not given again.
- The log and the inboxes are in memory. The journal file is a record only.
  The server does not read it back at start.
- Claude Code loads MCP tools on demand, so the first turn of each agent
  spends one tool call to load the hikari tools.
