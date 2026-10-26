-include .env
export

COMPOSE ?= examples/studio/compose.yaml
LINUX_ARCHS = amd64 arm64
UI_SRC = $(shell find web/src web/public -type f) web/index.html web/package.json web/vite.config.ts web/tsconfig.json web/tsconfig.app.json
unexport UI_SRC

.PHONY: example-onboard build install ui image up dev-ui test clean demo-repo example-sensors

# The Linux binaries are what hikari mounts into each agent's container.
build: web/dist/index.html
	go build -o bin/hikari ./cmd/hikari
	for arch in $(LINUX_ARCHS); do \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 go build -o bin/hikari-linux-$$arch ./cmd/hikari || exit 1; \
	done

# Install the binaries in /opt/hikari/bin, on the system PATH; see install.sh.
install:
	./install.sh

ui: web/dist/index.html

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	touch web/node_modules

web/dist/index.html: web/node_modules $(UI_SRC)
	cd web && npm run build

image:
	docker build -t hikari-agent .

up: build image
	bin/hikari up $(COMPOSE)

# Serve the UI with hot reload on http://localhost:5173 against a running `make up`.
dev-ui: web/node_modules
	cd web && npm run dev

test: web/node_modules
	go test -race ./...
	cd web && npm test

# Create the repo of examples/shop in .hikari/demo/shop, and reset its store
# in examples/shop/store with a sample shared policy.
demo-repo:
	examples/shop/make-repo.sh .hikari/demo/shop
	rm -rf examples/shop/store
	mkdir -p examples/shop/store
	cp -R examples/shop/seed/. examples/shop/store/

# Make the sensors example's project repository in .hikari/demo/sensors,
# replacing what is there.
example-sensors:
	rm -rf .hikari/demo/sensors
	mkdir -p .hikari/demo
	cp -R examples/sensors/project .hikari/demo/sensors
	git -C .hikari/demo/sensors init --quiet --initial-branch=main
	git -C .hikari/demo/sensors add --all
	git -C .hikari/demo/sensors -c user.name="hikari demo" -c user.email=demo@hikari.invalid commit --quiet -m "Start the sensors project"
	@echo "made the sensors project in .hikari/demo/sensors; run: make up COMPOSE=.hikari/demo/sensors/compose.yaml"

# Remove containers and networks left by runs that did not shut down cleanly.
example-onboard: build
	rm -rf .hikari/demo/onboard
	mkdir -p .hikari/demo/onboard/remotes
	for r in server cli; do \
		cp -R examples/onboard/$$r .hikari/demo/onboard/src-$$r && \
		git -C .hikari/demo/onboard/src-$$r init --quiet --initial-branch=main && \
		git -C .hikari/demo/onboard/src-$$r add --all && \
		git -C .hikari/demo/onboard/src-$$r -c user.name="hikari demo" -c user.email=demo@hikari.invalid commit --quiet -m "Start $$r" && \
		git clone --quiet --bare .hikari/demo/onboard/src-$$r .hikari/demo/onboard/remotes/$$r.git && \
		rm -rf .hikari/demo/onboard/src-$$r || exit 1; \
	done
	bin/hikari init -name onboard .hikari/demo/onboard/project
	@echo "the remotes are file://$(CURDIR)/.hikari/demo/onboard/remotes/server.git and cli.git"

clean:
	-docker rm -f $$(docker ps -aq --filter label=hikari.run) 2>/dev/null
	-docker network rm $$(docker network ls -q --filter label=hikari.run) 2>/dev/null
