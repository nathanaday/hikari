# Design notes

This document gives the reasons for the main decisions in hikari.

## One hub routes all messages

Every message goes through the hub. The hub knows the topology, checks each
message against the edges, and records each message in the log.

The other option was to connect neighbors directly, with one Docker network
for each edge. That design spreads the topology across Docker networks, makes
the log a copy of traffic, and lets an agent with a shell reach any service
on a shared network. With one hub, there is one place to check access and
one log that is always complete.

## Identity comes from a token

The hub gives each container a random token at start. The hub finds the
sender from the token, never from the request body. An agent with a shell can
read its token and call the hub with `curl`, but it can act only as itself.
That is the same reach that its MCP tools give it.

## Containers cannot reach each other

Each container has its own bridge network. Containers can reach the internet,
because Claude Code needs the Anthropic API, and the hub on the host. They
cannot reach each other.

## The UI needs its own secret

Containers can reach the hub, so the hub cannot trust a request because it
arrives at the hub address. The UI endpoints need a secret that the hub prints
at start, as Jupyter does. Without the secret, an agent could read the full
log or send messages as the user.

## Agents run headless, one turn per batch of messages

The driver in each container waits for messages. When messages arrive, it
runs `claude -p --resume <session>` with the messages as the prompt. The
agent keeps its context across turns because every turn resumes the same
session. Messages that arrive during a turn wait for the next turn, or the
agent reads them with `check_inbox`.

Two other options exist:

- An interactive session that calls a blocking `wait_for_message` tool in a
  loop. The model can stop the loop, and a long tool call can time out.
- Claude Code channels, which push MCP notifications into a live session.
  This feature is a research preview. It is a good candidate when it becomes
  stable, because it removes the per-turn process start.

The headless loop is predictable, and its output (stream-json) also gives
the driver tool calls and cost for each turn.

## The hub runs on the host

The hub must start containers. A hub in a container needs the Docker socket,
which gives full control of the host. On the host, the hub is a plain process
that is easy to run and to debug.

On Docker Desktop, containers reach a host service on 127.0.0.1 through
`host.docker.internal`. On Linux, `host.docker.internal` maps to the Docker
bridge address. There, start the hub with `-listen 0.0.0.0:7420`, or listen
on the bridge address, so that containers can reach it.

## Known limitations

- Two agents can send messages to each other without end. The system prompt
  tells agents not to send acknowledgments, but no hard limit exists.
- If a turn fails, the messages of that turn are not given again.
- The log and the inboxes are in memory. The journal file is a record only.
  The hub does not read it back at start.
