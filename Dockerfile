FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
RUN CGO_ENABLED=0 go build -o /out/hikari ./cmd/hikari

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git ripgrep \
 && rm -rf /var/lib/apt/lists/*
RUN useradd --create-home --shell /bin/bash agent
USER agent
RUN curl -fsSL https://claude.ai/install.sh | bash
ENV PATH=/home/agent/.local/bin:$PATH
COPY --from=build /out/hikari /usr/local/bin/hikari
WORKDIR /home/agent/workspace
ENTRYPOINT ["hikari", "node"]
