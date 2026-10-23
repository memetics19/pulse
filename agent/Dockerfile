# Build context: pulse/ (workspace root).
# The agent is a standalone Go module; build it with the workspace disabled
# (GOWORK=off) so it does not need the api/ or cli/ modules to be present.
FROM golang:1.27.3-alpine AS builder
ENV GOTOOLCHAIN=auto GOWORK=off CGO_ENABLED=0
WORKDIR /src

# Dependency manifests first (cache layer).
COPY agent/go.mod agent/go.sum ./
RUN go mod download

# Full agent source.
COPY agent/ ./

# Build the static binary.
RUN go build -trimpath -ldflags "-s -w" -o /pulse-agent ./cmd/agent

FROM alpine:3.24
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 agent
COPY --from=builder /pulse-agent /usr/local/bin/pulse-agent
USER agent
# The agent listens on nothing (it pushes to the server), so there is nothing to probe. A
# stalled agent shows up on the server as a host whose metrics stopped.
HEALTHCHECK NONE
ENTRYPOINT ["/usr/local/bin/pulse-agent"]
