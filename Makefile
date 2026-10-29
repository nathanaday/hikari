-include .env
export

TOPOLOGY ?= examples/shop/topology.yaml
LINUX_ARCHS = amd64 arm64
UI_SRC = $(shell find web/src web/public -type f) web/index.html web/package.json web/vite.config.ts web/tsconfig.json web/tsconfig.app.json
# The binary and the plugin share one version: plugin.json's.
VERSION := $(shell sed -n 's/.*"version": "\([^"]*\)".*/\1/p' plugin/.claude-plugin/plugin.json | head -1)
GO_LDFLAGS := -ldflags "-X github.com/nathanaday/hikari/internal/version.Version=$(VERSION)"
# A release builds with the toolchain of go.mod, so every machine makes the same bytes.
TOOLCHAIN := go$(shell sed -n 's/^go //p' go.mod)
RELEASE_FLAGS := -trimpath -buildvcs=false -ldflags "-s -w -X github.com/nathanaday/hikari/internal/version.Version=$(VERSION)"
PLATFORMS := darwin/arm64 darwin/amd64 linux/arm64 linux/amd64
SHA256 := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")
unexport UI_SRC VERSION GO_LDFLAGS TOOLCHAIN RELEASE_FLAGS PLATFORMS SHA256

.PHONY: example-onboard build install ui image up dev-ui test clean demo-repo release pin version skills

# The Linux binaries are what hikari mounts into each agent's container.
build: web/dist/index.html
	go build $(GO_LDFLAGS) -o bin/hikari ./cmd/hikari
	for arch in $(LINUX_ARCHS); do \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 go build $(GO_LDFLAGS) -o bin/hikari-linux-$$arch ./cmd/hikari || exit 1; \
	done

# Install this checkout's build where the plugin's launcher runs it, in
# ~/.hikari/bin/<version>/, with ~/.hikari/bin on the system PATH; see install.sh.
install:
	./install.sh

# release builds the binary of each platform into build/release, with its checksums.
release: web/dist/index.html
	rm -rf build/release && mkdir -p build/release
	for p in $(PLATFORMS); do \
		CGO_ENABLED=0 GOTOOLCHAIN=$(TOOLCHAIN) GOOS=$${p%/*} GOARCH=$${p#*/} \
			go build $(RELEASE_FLAGS) -o build/release/hikari-$(VERSION)-$${p%/*}-$${p#*/} ./cmd/hikari || exit 1; \
	done
	cd build/release && $(SHA256) hikari-* > checksums.txt

# pin copies the checksums into the plugin and writes the launcher that holds them.
pin: release
	mkdir -p plugin/release
	cp build/release/checksums.txt plugin/release/checksums.txt
	go run ./internal/release/pin

# skills copies the references that hikify borrows from the agents' skills into the
# plugin; a test fails when a copy differs from its source.
skills:
	cp internal/driver/skills/partition/hikari-map-features/references/wiki.md plugin/skills/hikify/references/wiki.md
	cp internal/driver/skills/orchestration/hikari-map-features/SKILL.md plugin/skills/hikify/references/orchestration.md

# version sets V as the version of the plugin and of its marketplace entry, then pins
# the binaries of that version. Commit, then push the tag V to publish the release.
version:
	@test -n "$(V)" || (echo "Usage: make version V=<major.minor.patch>" && exit 1)
	go run ./internal/release/pin -set $(V)
	$(MAKE) pin

ui: web/dist/index.html

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	touch web/node_modules

web/dist/index.html: web/node_modules $(UI_SRC)
	cd web && npm run build

# The agent image comes from the Dockerfile that the binary embeds; see hikari image.
image: build
	bin/hikari image

up: build image
	@if [ "$(TOPOLOGY)" = examples/shop/topology.yaml ] && [ ! -d .hikari/demo/shop ]; then \
		echo "the shop's repo is missing: run make demo-repo first"; exit 1; \
	fi
	bin/hikari up $(TOPOLOGY)

# Serve the UI with hot reload on http://localhost:5173 against a running `make up`.
dev-ui: web/node_modules
	cd web && npm run dev

test: web/node_modules
	go test -race ./...
	cd web && npm test

# Create the repo of examples/shop in .hikari/demo/shop, and reset its vault
# in examples/shop/vault from the seed: a policy, a deliverable, and a skill.
demo-repo:
	examples/shop/make-repo.sh .hikari/demo/shop
	rm -rf examples/shop/vault
	mkdir -p examples/shop/vault
	cp -R examples/shop/seed/. examples/shop/vault/
	rm -rf "$${SHOP_RELEASES:-$$HOME/.hikari/shop-releases}"

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

# Remove containers and networks left by runs that did not shut down cleanly.
clean:
	-docker rm -f $$(docker ps -aq --filter label=hikari.run) 2>/dev/null
	-docker network rm $$(docker network ls -q --filter label=hikari.run) 2>/dev/null
