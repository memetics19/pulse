# Build context: pulse/ (workspace root)
FROM golang:1.25rc1-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /workspace

# Copy workspace manifests first (cache layer)
COPY go.work go.work.sum ./

# Copy each module's dependency manifests for better layer caching
COPY agent/go.mod agent/go.sum ./agent/
COPY api/go.mod api/go.sum ./api/
COPY worker/go.mod worker/go.sum ./worker/

# Download deps (workspace-aware)
RUN go work sync && go mod download -modfile agent/go.mod

# Copy full source
COPY agent/ ./agent/
COPY api/ ./api/
COPY worker/ ./worker/

# Build the agent binary (CGO disabled → fully static)
RUN CGO_ENABLED=0 go build -o /pulse-agent ./agent/cmd/agent

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /pulse-agent /usr/local/bin/pulse-agent
ENTRYPOINT ["/usr/local/bin/pulse-agent"]
