-include .env
export

TOPOLOGY ?= examples/studio.yaml
UI_SRC = $(shell find web/src web/public -type f) web/index.html web/package.json web/vite.config.ts web/tsconfig.json web/tsconfig.app.json
unexport UI_SRC

.PHONY: build ui image up dev-ui test clean demo-repo

build: web/dist/index.html
	go build -o bin/hikari ./cmd/hikari

ui: web/dist/index.html

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	touch web/node_modules

web/dist/index.html: web/node_modules $(UI_SRC)
	cd web && npm run build

image:
	docker build -t hikari-node .

up: build image
	bin/hikari up $(TOPOLOGY)

# Serve the UI with hot reload on http://localhost:5173 against a running `make up`.
dev-ui: web/node_modules
	cd web && npm run dev

test: web/node_modules
	go test -race ./...
	cd web && npm test

# Create the repo of examples/systems.yaml in .hikari/demo/shop, and reset
# its store in examples/store with a sample shared policy.
demo-repo:
	examples/demo/make-repo.sh .hikari/demo/shop
	rm -rf examples/store
	mkdir -p examples/store
	cp -R examples/demo/store/. examples/store/

# Remove containers and networks left by runs that did not shut down cleanly.
clean:
	-docker rm -f $$(docker ps -aq --filter label=hikari.run) 2>/dev/null
	-docker network rm $$(docker network ls -q --filter label=hikari.run) 2>/dev/null
