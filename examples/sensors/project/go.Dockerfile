FROM hikari-agent
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends golang-go go-junit-report \
 && rm -rf /var/lib/apt/lists/*
USER agent
ENV GOPATH=/tmp/go GOCACHE=/tmp/go-cache GOFLAGS=-mod=mod
