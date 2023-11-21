SHELL := /usr/bin/env bash

.PHONY: build
build:
	@command -v task >/dev/null || (echo "task not found, please install it. https://taskfile.dev/installation/" && exit 1)
	@task