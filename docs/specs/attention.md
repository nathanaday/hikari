# Spec: Attention, Decisions, and the Grid

Status: built. Checked with `go test -race ./...`, `npm test`, and `npm run build`, and rendered in a live `hikari up -no-launch` on the shop example.

Source: a review of a multi-agent terminal UI on 2026-11-04, from which three features carry over: an inbox of what waits for the user, a decision card with numbered choices, and a grid of every session.

## Summary

Hikari gets one place that says what is blocked on you, a message kind that asks you a question with numbered choices and takes your answer with one click or one key, and a view that shows every agent as a tile.

## Decisions

1. **The inbox is computed, not stored.** Every item comes from state the UI already has: the messages, the tasks, the hikari agent's prompts, the repository views, and the trackers. Nothing new is persisted, and nothing can go stale.
2. **A decision is a message.** It travels, journals, prints in the terminal, and reaches the feed as any direct message does; the UI alone draws it as a card. The answer is a direct message back, "`<n>. <choice>`" with the user's words, that names the decision it answers, so the asker's turn starts as it would from any message.
3. **Hikari asks too.** The pipeline raises a decision when the orchestration agent scopes a queued task with assumptions, and when a scope is blocked; the answer goes to the orchestration agent, which acts on it.
4. **The grid replaces nothing.** All activity, the direct messages, and the map stay; the grid is the overview for a large topology.

## Requirements

- **AT1.** An item of the inbox is: a decision that waits for an answer; an agent's last direct message to the user with nothing of the user's after it; a hikari agent's permission not yet answered; a task that is `blocked`; an open pull request whose checks fail or that conflicts; a repository whose forge or fetch errored; a tracker that could not be read.
- **AT2.** Each item opens the view that resolves it: the direct messages, the issue, the repository, or the Work tab.
- **AT3.** The sidebar lists the items in a "Needs you" group at the top, with the count, and marks each agent's and repository's entry with the number of its items. The group is absent when nothing waits.
- **DC1.** A direct message may carry a decision: a question and two to six options, each not empty. A decision goes to the user only. Its body is the decision's text when the sender gives none.
- **DC2.** `ask_user(question, options, task)` is every agent's tool. A module asks through `Host.Ask`, as `hikari`, naming who receives the answer.
- **DC3.** `POST /api/ui/decide {message, choice, note}` sends the answer as the user: a direct message to the decision's `reply_to`, "`<n>. <option>`" and the note on a new line, with `answers` set to the decision's id. A choice outside the options, or a message that is no decision, is refused.
- **DC4.** The pipeline asks: "scoped with assumptions, proceed?" with Proceed and Change the scope; and "blocked, what now?" with Resolved, Skip this part, and Keep it blocked. Both go to the orchestration agent. A block set by the user asks nothing.
- **DC5.** The UI draws a decision as a card with numbered buttons and an optional field for words; the latest waiting decision in the open conversation takes the digit keys. Once answered, the card shows the pick. An issue's panel shows the decisions of its task.
- **GR1.** `#/grid` shows one tile per agent: its status, the step it is on or its last turn's summary, the tasks it holds, its last message, and a mark when it waits for the user. A tile opens the agent's direct messages.
- **GR2.** A partition agent holds the started scopes with a part for it, with its part's status; the orchestration agent holds every started scope.
