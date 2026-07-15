# Build context: pulse/ (workspace root).
# The agent is a standalone Go module; build it with the workspace disabled
# (GOWORK=off) so it does not need the api/ or cli/ modules to be present.
FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto GOWORK=off CGO_ENABLED=0
WORKDIR /src

# Dependency manifests first (cache layer).
COPY agent/go.mod agent/go.sum ./
RUN go mod download

# Full agent source.
COPY agent/ ./

# Build the static binary.
RUN go build -o /pulse-agent ./cmd/agent

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /pulse-agent /usr/local/bin/pulse-agent
ENTRYPOINT ["/usr/local/bin/pulse-agent"]
