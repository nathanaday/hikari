# Fixing what hikari doctor reports

`hikari doctor` prints `ok`, `!` for a warning, or `✗` for a problem that blocks `hikari up`. A line that needs `sudo`, a password, or an install is the user's to run: give them the exact command, ask them to run it in their own terminal, and run doctor again when they say it is done.

| Doctor says | What to do |
|---|---|
| hikari is not installed yet, or a download failed | The launcher downloads hikari from its GitHub release on the first command. Check the network with the user, and run doctor again. A user with a build of their own sets `HIKARI_BIN` to it. |
| a Linux binary for the containers is missing | The install is incomplete. Ask the user to remove the folder that doctor names, then run doctor again, which installs it anew. In a checkout of hikari, `make install` does the same. |
| the folder is not on the PATH of your terminal | The user needs `hikari` in their own terminal to run `hikari up`. Give them the line that doctor prints; `sudo` asks for their password once. A new terminal then finds `hikari`. |
| an older install is in `/opt/hikari` | Give the user the line that doctor prints. |
| the hikari that the PATH finds first is another version | An older install comes before the plugin's. Until the user removes it, run every hikari command of the skills by the path of the plugin's launcher, which the hikari skill ran doctor with. |
| docker is not installed | Ask the user to install Docker Desktop on macOS, or Docker Engine with the compose plugin on Linux. Stop until they have. |
| Docker does not answer | Ask the user to start Docker Desktop, or the Docker daemon. Run doctor again. |
| the `hikari-agent` image is not built, or comes from an older Dockerfile | `hikari up` builds it on its next start. To build it now, ask the user, then run `hikari image`. It downloads Debian and Claude Code, which takes a few minutes. |
| git has no `user.name` or `user.email` | The skills commit the project as the user. Ask for their name and email, and with their approval run `git config --global user.name "<name>"` and `git config --global user.email "<email>"`. |
| no credential for the agents | Step 5 of "Make the project" in the hikari skill. Never handle the credential yourself. |
| the topology file has errors | `hikari check topology.yaml` names each one. During onboarding, the skill of the stage fixes them. |
