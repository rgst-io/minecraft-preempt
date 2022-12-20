# syntax=docker/dockerfile:1.0-experimental
FROM golang:1.19.4-alpine3.17 AS builder
ARG VERSION
ENV GOCACHE "/go-build-cache"
ENV CGO_ENABLED 0
WORKDIR /src

# Copy our source code into the container for building
COPY . .

# Cache dependencies across builds
RUN --mount=type=ssh --mount=type=cache,target=/go/pkg go mod download

# Build our application, caching the go build cache, but also using
# the dependency cache from earlier.
RUN --mount=type=ssh --mount=type=cache,target=/go/pkg --mount=type=cache,target=/go-build-cache \
  mkdir -p bin; \
  go build -o /src/bin/ -ldflags "-s -w" -v ./

FROM scratch
LABEL org.opencontainers.image.source https://github.com/jaredallard/minecraft-preempt
ENTRYPOINT ["/usr/local/bin/minecraft-preempt"]

# Certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Binary
COPY --from=builder /src/bin/minecraft-preempt /usr/local/bin/minecraft-preempt 