# go options
GO         ?= go
PKG        := go mod download
LDFLAGS    := -w -s
BINDIR     := $(CURDIR)/bin

# Required for globs to work correctly
SHELL=/bin/bash

.PHONY: all
all: build

.PHONY: dep
dep:
	@$(PKG)

.PHONY: build
build:
	CGO_ENABLED=0 $(GO) build -v -o $(BINDIR)/ -ldflags '$(LDFLAGS)' ./cmd/...

.PHONY: reload
reload:
	@echo "Watching for changes to .go files..."
	@go run github.com/cespare/reflex@latest --regex='\.go$$' --decoration=fancy --start-service=true bash -- -c 'make build && exec ./bin/minecraft-preempt'