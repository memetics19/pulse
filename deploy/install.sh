#!/bin/sh
# Pulse installer.
#
#   curl -fsSL https://raw.githubusercontent.com/memetics19/pulse/main/deploy/install.sh | sh
#
# Pulse is a single static Go binary with the admin UI embedded and a SQLite
# database, so there is no Docker, Node.js, or database server to install. This
# script detects your OS/arch, downloads the matching binary, verifies its
# checksum, and installs it to /usr/local/bin. On Linux with systemd it also
# sets up an auto-starting `pulse` service; on macOS it prints the run command.
set -eu

REPO="memetics19/pulse"
BIN="pulse"

say()  { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- terminal for prompts (stdin is the script itself under `curl | sh`) -------
if [ -t 0 ]; then TTY_IN=""; INTERACTIVE=1
elif [ -r /dev/tty ]; then TTY_IN=/dev/tty; INTERACTIVE=1
else TTY_IN=""; INTERACTIVE=0
fi

ask() { # ask VAR "Prompt" "default"
    _var="$1"; _prompt="$2"; _def="$3"
    if [ "$INTERACTIVE" -eq 0 ]; then eval "$_var=\$_def"; return; fi
    printf '%s [%s]: ' "$_prompt" "$_def" >&2
    if [ -n "$TTY_IN" ]; then read -r _ans < "$TTY_IN" || _ans=""
    else read -r _ans || _ans=""; fi
    [ -n "$_ans" ] || _ans="$_def"
    eval "$_var=\$_ans"
}

askbool() { # askbool VAR "Prompt" "yes|no"
    ask "$1" "$2 (yes/no)" "$3"
    eval "_v=\$$1"
    case "$_v" in y|Y|yes|YES|true|1) eval "$1=true" ;; *) eval "$1=false" ;; esac
}

# --- detect OS + arch ----------------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    linux|darwin) ;;
    *) fail "unsupported OS '$OS' (supported: Linux, macOS)" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) fail "unsupported architecture '$ARCH' (supported: x86_64/amd64, arm64/aarch64)" ;;
esac
ASSET="${BIN}_${OS}_${ARCH}"

# --- downloader (curl or wget) -------------------------------------------------
if command -v curl >/dev/null 2>&1; then
    dl() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
    dl() { wget -qO "$2" "$1"; }
else
    fail "need curl or wget installed to download the binary"
fi

VERSION="${PULSE_VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
    BASE="https://github.com/${REPO}/releases/latest/download"
else
    BASE="https://github.com/${REPO}/releases/download/${VERSION}"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

say "Pulse installer"
say "Platform: ${OS}/${ARCH}   Version: ${VERSION}"
say ""
say "Downloading ${ASSET} ..."
dl "${BASE}/${ASSET}" "${TMP}/${BIN}" \
    || fail "could not download ${BASE}/${ASSET} (no release binary for ${OS}/${ARCH} yet?)"

# --- verify checksum (best effort; older releases may lack SHA256SUMS) ---------
if dl "${BASE}/SHA256SUMS" "${TMP}/SHA256SUMS" 2>/dev/null; then
    want="$(awk -v f="$ASSET" '$2==f {print $1}' "${TMP}/SHA256SUMS")"
    if [ -n "$want" ]; then
        if command -v sha256sum >/dev/null 2>&1; then
            got="$(sha256sum "${TMP}/${BIN}" | awk '{print $1}')"
        elif command -v shasum >/dev/null 2>&1; then
            got="$(shasum -a 256 "${TMP}/${BIN}" | awk '{print $1}')"
        else
            got=""
        fi
        if [ -n "$got" ] && [ "$got" != "$want" ]; then
            fail "checksum mismatch for ${ASSET} (expected ${want}, got ${got})"
        fi
        [ -n "$got" ] && say "Checksum verified."
    fi
else
    warn "no SHA256SUMS published for this release; skipping checksum verification"
fi
chmod +x "${TMP}/${BIN}"

# --- sudo helper ---------------------------------------------------------------
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then SUDO="sudo"
    else warn "not running as root and sudo is missing; install steps may fail"; fi
fi

INSTALL_PATH="/usr/local/bin/${BIN}"
say "Installing to ${INSTALL_PATH} ..."
$SUDO install -m 0755 "${TMP}/${BIN}" "${INSTALL_PATH}"

# --- settings ------------------------------------------------------------------
say ""
[ "$INTERACTIVE" -eq 0 ] && say "(no terminal detected; using defaults)"
ask PORT "Port to serve Pulse on" "8080"
case "$PORT" in ''|*[!0-9]*) fail "port must be a number" ;; esac
askbool SECURE "Serve behind HTTPS? (marks session cookies Secure)" "no"
askbool PRIVATE "Allow monitors to target private/LAN addresses? (homelab)" "no"

write_env() { # write_env <path> ; caller sets $DATADIR
    cat <<EOF
API_PORT=${PORT}
SQLITE_PATH=${DATADIR}/pulse.db
PULSE_DATA_DIR=${DATADIR}
PULSE_SECURE_COOKIES=${SECURE}
PULSE_ALLOW_PRIVATE_MONITORS=${PRIVATE}
RESEND_API_KEY=
SLACK_WEBHOOK_URL=
EOF
}

# --- Linux + systemd: install an auto-starting service -------------------------
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    ask DATADIR "Data directory" "/var/lib/pulse"
    ENVFILE="/etc/pulse/pulse.env"

    say "Creating the pulse service user and directories ..."
    if ! id pulse >/dev/null 2>&1; then
        $SUDO useradd --system --no-create-home --shell /usr/sbin/nologin pulse 2>/dev/null \
            || $SUDO adduser --system --no-create-home --shell /sbin/nologin pulse 2>/dev/null \
            || $SUDO adduser -S -H pulse 2>/dev/null \
            || warn "could not create the 'pulse' user; the service will run as root"
    fi
    $SUDO mkdir -p "$DATADIR" /etc/pulse
    $SUDO chown -R pulse "$DATADIR" 2>/dev/null || true

    write_env | $SUDO tee "$ENVFILE" >/dev/null
    $SUDO chmod 640 "$ENVFILE"

    RUN_USER="pulse"
    id pulse >/dev/null 2>&1 || RUN_USER="root"
    $SUDO tee /etc/systemd/system/pulse.service >/dev/null <<EOF
[Unit]
Description=Pulse status page
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${RUN_USER}
EnvironmentFile=${ENVFILE}
ExecStart=${INSTALL_PATH}
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
PrivateTmp=true
ReadWritePaths=${DATADIR}

[Install]
WantedBy=multi-user.target
EOF

    say "Starting the pulse service ..."
    $SUDO systemctl daemon-reload
    $SUDO systemctl enable --now pulse

    say ""
    say "Pulse is running as a systemd service. Finish setup in your browser:"
    say ""
    say "    http://localhost:${PORT}/setup"
    say ""
    say "Manage it:"
    say "    sudo systemctl status pulse       # health"
    say "    sudo journalctl -u pulse -f       # logs"
    say "    sudo systemctl restart pulse      # restart"
    say ""
    say "Config: ${ENVFILE}    Data: ${DATADIR}"
    exit 0
fi

# --- macOS / no systemd: install the binary and print the run command ----------
ask DATADIR "Data directory" "${HOME}/.pulse"
mkdir -p "$DATADIR"
ENVFILE="${DATADIR}/pulse.env"
write_env > "$ENVFILE"

say ""
say "Pulse is installed at ${INSTALL_PATH}."
say "Settings written to ${ENVFILE}."
say ""
say "Start Pulse with:"
say ""
say "    env \$(grep -v '^#' \"${ENVFILE}\" | xargs) ${BIN}"
say ""
say "Then finish setup at http://localhost:${PORT}/setup"
say "To run it on boot, wire that command into launchd or your process manager."
