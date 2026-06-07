# syntax=docker/dockerfile:1

# 1. Build the admin UI as a static export.
FROM node:20-alpine AS ui
WORKDIR /ui
COPY ui/package*.json ./
RUN NEXT_PUBLIC_API_URL="" npm ci
COPY ui/ ./
RUN NEXT_PUBLIC_API_URL="" npm run build

# 2. Build the single Go binary, embedding the admin export.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY agent/ agent/
COPY api/ api/
COPY cli/ cli/
COPY --from=ui /ui/out/ api/internal/web/dist/
RUN cd api && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/pulse ./cmd/pulse

# 3. Minimal runtime: one static binary plus a SQLite file.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 pulse \
 && mkdir -p /data \
 && chown pulse:pulse /data
COPY --from=build /out/pulse /usr/local/bin/pulse
USER pulse
ENV SQLITE_PATH=/data/pulse.db \
    PULSE_DATA_DIR=/data \
    API_PORT=8080
EXPOSE 8080
VOLUME /data
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1
ENTRYPOINT ["pulse"]
