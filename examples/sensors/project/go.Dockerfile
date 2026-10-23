FROM hikari-agent
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends golang-go ca-certificates git \
 && rm -rf /var/lib/apt/lists/* \
 && GOBIN=/usr/local/bin GOPATH=/tmp/gopath go install github.com/jstemmer/go-junit-report/v2@v2.1.0 \
 && rm -rf /tmp/gopath /root/.cache
USER agent
ENV GOPATH=/tmp/go GOCACHE=/tmp/go-cache
