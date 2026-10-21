-include .env
export

TOPOLOGY ?= examples/studio.yaml

.PHONY: build image up test clean

build:
	go build -o bin/hikari ./cmd/hikari

image:
	docker build -t hikari-node .

up: build image
	bin/hikari up $(TOPOLOGY)

test:
	go test -race ./...

# Remove containers and networks left by runs that did not shut down cleanly.
clean:
	-docker rm -f $$(docker ps -aq --filter label=hikari.run) 2>/dev/null
	-docker network rm $$(docker network ls -q --filter label=hikari.run) 2>/dev/null
