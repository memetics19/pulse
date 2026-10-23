# syntax=docker/dockerfile:1

# The build stages run on the build machine's own platform ($BUILDPLATFORM) and Go
# cross-compiles for the target, so a multi-arch build emulates only the runtime stage.

# 1. Build the admin UI as a static export. The output is plain files, the same on
#    every architecture, so this stage runs once for all targets.
FROM --platform=$BUILDPLATFORM node:24-alpine AS ui
WORKDIR /ui
COPY ui/package*.json ./
RUN NEXT_PUBLIC_API_URL="" npm ci
COPY ui/ ./
RUN NEXT_PUBLIC_API_URL="" npm run build

# 2. Build the single Go binary for the target platform, embedding the admin export.
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY agent/ agent/
COPY api/ api/
COPY cli/ cli/
COPY --from=ui /ui/out/ api/internal/web/dist/
WORKDIR /src/api
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/pulse ./cmd/pulse

# 3. Minimal runtime: one static binary plus a SQLite file.
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 pulse \
 && mkdir -p /data \
 && chown pulse:pulse /data
COPY --from=build /out/pulse /usr/local/bin/pulse
USER pulse
# PULSE_PORT (8080) and PULSE_DB (/data/pulse.db) keep their built-in defaults.
# Setting them here would hide API_PORT and SQLITE_PATH from older deployments,
# which Pulse still reads with a warning until 4.0.
ENV PULSE_DATA_DIR=/data
EXPOSE 8080
VOLUME /data
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1
ENTRYPOINT ["pulse"]
