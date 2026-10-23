FROM hikari-agent
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python3-pytest curl \
 && rm -rf /var/lib/apt/lists/*
USER agent
