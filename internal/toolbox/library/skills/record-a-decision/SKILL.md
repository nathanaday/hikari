---
name: record-a-decision
description: Write down a decision about your partition so that it outlives the conversation - the choice, its reasons, and the options you rejected - as a principle or a note of your feature map through change_wiki. Load it when you chose between designs, when the user settled a question, or when a reviewer asks why.
---

# Record a decision

A decision that lives only in a message is lost at the next turn. Put it in the feature map, where every later turn reads it.

1. **Is it a rule or a fact?** A decision every change must keep, such as "money is integer cents", is a `principle`. A decision that explains the present shape, such as "we chose SQLite for the demo", is a `note`.
2. **Write it** with `change_wiki`, kind `principle` or `note`, in this shape:
   - **Decision**: one sentence.
   - **Why**: the forces: the constraint, the user's words, the failing test, the contract.
   - **Rejected**: each option you did not take, with the reason in one line.
   - **Consequences**: what gets easier, what gets harder, what to revisit and when.
3. **Link it** to the features it binds with `[[id]]`, so that the feature's page shows it.
4. **Say it** in your reply: the id of the document, so that the user and the reviewer can read it.

Do not record a decision you did not make, and never change a plan or a note the user wrote.
