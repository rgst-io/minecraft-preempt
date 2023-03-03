# go option
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
	CGO_ENABLED=0 $(GO) build -o $(BINDIR)/ -ldflags '$(LDFLAGS)' ./cmd/...
