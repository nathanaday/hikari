---
name: hikari-review-work
description: How a hikari partition agent has its change reviewed before it says that the work is done - read the whole change with get_changes, send the fresh hikari-reviewer agent with the diff, the scope's part, and the styles and policies, fix what it finds, and carry its verdict into set_task_status as the review. Load it before you ask for done on a part when your prompt says your work is reviewed, and whenever a change is large or touches an interface.
---

# Have the work reviewed

A review is a second reading by someone who did not write the change. The reviewer is a fresh subagent, `hikari-reviewer`, with no memory of your turn: it reads the diff against the rules you were given and reports what breaks them. You decide what to do with each finding, and the verdict goes with your request for done.

1. **Finish first.** The suites pass and are recorded, the documents are true, and the change is applied. A review of half a change wastes the reviewer.
2. **Read the change** with `get_changes`: the diff of your folders in each repository, from the merge base with the default branch, and the files git does not track yet. Read it yourself once. Fix what you see before anyone else does.
3. **Send the reviewer.** Send the `hikari-reviewer` agent with:
   - the change, from `get_changes`, or the paths to read when the diff is long;
   - what the change was for: the part of the scope from `get_task`, or the request in the user's words;
   - the rules: `/hikari/public/policies/` and `/hikari/styles/`, which hold the policies and the styles you were given, and the feature documents in `/hikari/self/features/` that the change touches.

   It returns findings as JSON: each with a severity, a place, what it saw, and the rule or the reason.
4. **Act on the findings.**
   - `blocker`: fix it, run the suites that touch it, record the run, and apply. A blocker you disagree with is a question for the user, with `ask_user`; never silently drop one.
   - `should`: fix it when it is small; otherwise note it in your report as left undone, with why.
   - `nit`: your call. Say in one line what you took and what you left.
5. **Review again** when a blocker's fix was more than a line or two. Two rounds are normal; a third means the change is too large, so say so.
6. **Carry the verdict.** When you ask for done with `set_task_status`, pass `review`: one line that says who reviewed, how many findings of each severity, what you fixed, and what you left. hikari records it in the task's log, where the user reads it. When your prompt says your work is reviewed, hikari refuses done without it.

What a review is not: it is not a test run, which `hikari-validate-work` does; it is not a check of the health, which hikari computes; and it is not approval, which is the user's. It is a reading.
