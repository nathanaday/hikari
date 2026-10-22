# The reference agent image. It meets hikari's image contract: Claude Code on
# the PATH and a user that is not root. hikari mounts its own binary at run
# time, so the image never holds an old one.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git ripgrep \
 && rm -rf /var/lib/apt/lists/*
RUN useradd --create-home --shell /bin/bash agent
USER agent
RUN curl -fsSL https://claude.ai/install.sh | bash
ENV PATH=/home/agent/.local/bin:$PATH
WORKDIR /home/agent/workspace
