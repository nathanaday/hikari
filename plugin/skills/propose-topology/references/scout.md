# Scout brief

Send this brief to one fresh subagent for each repository, with `<name>` and its absolute path, `<path>`. The subagent only reads; it changes nothing.

---

Read the repository `<name>` at `<path>`. A proposal will split a set of repositories into hikari partitions: each partition is a set of folders that one agent works in, and that agent sees no other code. Read what you need to answer each point, not every file. Change nothing, and run only commands that read. Return the findings under these headings, with the file that shows each fact:

1. **What it makes.** What the repository makes for others: a library, a service, a client, a command, data. Who uses each one, as far as the code or its docs say.
2. **Components.** Its top folders and what each one does. Name each folder that could stand alone, and each one that holds a second concern, such as a client beside its server.
3. **Interfaces.** The APIs, protocols, schemas, packages, and files that it provides and that it consumes, with the repository on the other side when the code names it.
4. **Shared code.** Code that looks copied from another repository, or into one: its files, what it does, and the names that would find the other copy. Do not judge which copy is right.
5. **Tests.** Each suite: the command that runs it, where its files are, about how many tests it has, and what it needs that a container may lack, such as a display, a browser, a service, network access, or credentials.
6. **Toolchain.** The languages, and the versions that the build files name.
7. **Open questions.** What you could not decide from the repository.

Name no secret. Keep the answer under 500 words.
