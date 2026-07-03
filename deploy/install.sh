#!/bin/sh
# Pulse installer: sets up the Pulse status page server with Docker.
#
#   curl -fsSL https://raw.githubusercontent.com/memetics19/pulse/main/deploy/install.sh | sh
#
# Prompts for a few settings (with sensible defaults), writes a
# docker-compose.yml + .env into the chosen directory, and starts Pulse.
set -eu

IMAGE="ghcr.io/memetics19/pulse:latest"

say()  { printf '%s\n' "$*"; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

# When run via `curl | sh`, stdin is the script itself, so prompts must read
# from the terminal. With no terminal at all, fall back to defaults.
INTERACTIVE=1
if [ -t 0 ]; then
    TTY_IN=""
elif [ -r /dev/tty ]; then
    TTY_IN="/dev/tty"
else
    INTERACTIVE=0
    TTY_IN=""
fi

ask() { # ask "Prompt" default -> stores answer in $answer
    prompt="$1"; default="$2"
    if [ "$INTERACTIVE" -eq 0 ]; then
        answer="$default"
        return
    fi
    printf '%s [%s]: ' "$prompt" "$default" >&2
    if [ -n "$TTY_IN" ]; then
        read -r answer < "$TTY_IN" || answer=""
    else
        read -r answer || answer=""
    fi
    [ -n "$answer" ] || answer="$default"
}

# --- preflight ---------------------------------------------------------------
command -v docker >/dev/null 2>&1 || fail "docker is not installed. See https://docs.docker.com/get-docker/"
docker compose version >/dev/null 2>&1 || fail "the docker compose plugin is missing. See https://docs.docker.com/compose/install/"
docker info >/dev/null 2>&1 || fail "the docker daemon is not reachable. Is it running, and does your user have permission?"

say "Pulse installer"
say ""
[ "$INTERACTIVE" -eq 0 ] && say "(no terminal detected; using defaults)"

# --- prompts ------------------------------------------------------------------
ask "Port to serve Pulse on" "8080"
PORT="$answer"
case "$PORT" in *[!0-9]*|"") fail "port must be a number" ;; esac

ask "Install directory (holds config + SQLite data)" "$HOME/pulse"
DIR="$answer"

ask "Serve over HTTPS behind a proxy? Marks session cookies Secure (yes/no)" "no"
case "$answer" in y|Y|yes|YES) SECURE="true" ;; *) SECURE="false" ;; esac

ask "Allow monitors to target private/LAN addresses? Needed for homelab monitoring (yes/no)" "no"
case "$answer" in y|Y|yes|YES) PRIVATE="true" ;; *) PRIVATE="false" ;; esac

# --- write files ---------------------------------------------------------------
mkdir -p "$DIR/data"

if [ -e "$DIR/docker-compose.yml" ]; then
    ask "$DIR/docker-compose.yml already exists. Overwrite? (yes/no)" "no"
    case "$answer" in y|Y|yes|YES) ;; *) fail "aborted; existing file left untouched" ;; esac
fi

cat > "$DIR/.env" <<EOF
PULSE_SECURE_COOKIES=$SECURE
PULSE_ALLOW_PRIVATE_MONITORS=$PRIVATE
RESEND_API_KEY=
SLACK_WEBHOOK_URL=
EOF
chmod 600 "$DIR/.env"

cat > "$DIR/docker-compose.yml" <<EOF
services:
  pulse:
    image: $IMAGE
    ports:
      - "$PORT:8080"
    environment:
      SQLITE_PATH: /data/pulse.db
      PULSE_SECURE_COOKIES: \${PULSE_SECURE_COOKIES:-false}
      PULSE_ALLOW_PRIVATE_MONITORS: \${PULSE_ALLOW_PRIVATE_MONITORS:-false}
      RESEND_API_KEY: \${RESEND_API_KEY:-}
      SLACK_WEBHOOK_URL: \${SLACK_WEBHOOK_URL:-}
    # Allow unprivileged ICMP (ping monitors) inside the container.
    sysctls:
      - net.ipv4.ping_group_range=0 2147483647
    volumes:
      - ./data:/data
    restart: unless-stopped
EOF

# --- start ---------------------------------------------------------------------
say ""
say "Starting Pulse from $DIR ..."
(cd "$DIR" && docker compose up -d)

say ""
say "Pulse is starting. Finish setup in your browser:"
say ""
say "    http://localhost:$PORT/setup"
say ""
say "Data lives in $DIR/data; manage the service with:"
say "    cd $DIR && docker compose logs -f   # logs"
say "    cd $DIR && docker compose down      # stop"
