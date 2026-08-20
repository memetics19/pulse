# Diagnostics

A diagnostic bundle is a read-only snapshot of a host's state, collected by
`pulse-agent` and stored by Pulse. It answers "why did this break" without
opening an SSH session — useful when the host is remote or hard to reach.

Today bundles are collected on demand with `pulse-agent --diagnose`. Attaching
them to incidents automatically is not yet implemented.

## What a bundle contains

| Section | Source | Contents |
| --- | --- | --- |
| `kernel` | `dmesg` | Out-of-memory kills, with the process, PID, and resident size |
| `disk` | `df -Pk` | Usage per mount, and which mounts are at capacity |
| `processes` | `ps` | The 15 busiest processes by CPU share |
| `docker` | `docker ps -a`, `docker logs` | All containers with state and exit status, which are not running, and recent output from those |
| `systemd` | `systemctl list-units --failed`, `journalctl` | Units in the failed state, and the recent journal for each |
| `proxmox` | `qm list` | Guest VMs and their status, and which are not running |

Every collector is read-only. The agent runs a fixed set of commands and never
executes anything supplied by the server.

`df` is invoked with `-k` deliberately: POSIX `-P` alone reports 512-byte blocks
on BSD and under `POSIXLY_CORRECT`, which would make `available_kb` double the
true value. The unit is part of the contract, not an implementation detail.

### Logs are captured only for what already failed

An OOM kill tells you a service died; its log tells you why. The agent pulls
logs only for units already in the failed state and containers already stopped —
it never takes a log target from the server.

Capture is bounded so a bundle stays sendable: the last 200 lines, for at most
5 failing units and 5 stopped containers, truncated to 32 KiB each. Truncated
logs keep their tail, since that is where the failure is, and are marked
`[truncated: log exceeded capture limit]`.

### Sections degrade independently

A host that denies `dmesg`, has no Docker, or is not a Proxmox node still
produces a useful bundle. Collectors that cannot run record their error in
their own section and leave the rest intact:

```json
{
  "sections": {
    "disk": { "data": { "mounts": [], "full": ["/"] } },
    "proxmox": { "error": "qm list: exec: \"qm\": executable file not found in $PATH" }
  }
}
```

Each command is individually timeout-bounded, so a wedged command cannot stall
collection — which matters most on exactly the hosts worth diagnosing.

A failing command's own output is folded into its section error, so the reason
survives rather than a bare exit status:

```
"error": "docker ps: exit status 1: failed to connect to the docker API ..."
"error": "dmesg: exit status 1: usage: sudo dmesg"
```

Reading the kernel ring buffer requires root on most systems. Run the agent as
root — as the systemd unit does — or the `kernel` section degrades.

Mounts on filesystems with no backing store are listed but never reported as
full, because they read 100% permanently: `devfs`, `devtmpfs`, `udev`, `proc`,
`sysfs`, and `efivarfs`.

Everything else is flagged when it reaches capacity, including `tmpfs`,
`overlay`, and loop-backed mounts. A full `tmpfs` is real memory-backed
exhaustion and a full `overlay` is a container's writable layer filling up, so
both are actionable.

Loop devices are a deliberate trade-off. A read-only image mount such as a snap
package sits at 100% permanently and will be reported as full, but `df -Pk`
names the device rather than the filesystem type, so it cannot be distinguished
from a writable loop-mounted ext4 or XFS volume. Reporting a harmless snap mount is the
lesser failure — hiding a genuinely full filesystem is a missed incident.

## Installing the agent

`pulse-agent` is published with each release for Linux and macOS on amd64 and
arm64. Install it on the host you want to diagnose:

```sh
VERSION=$(curl -fsSL https://api.github.com/repos/memetics19/pulse/releases/latest | grep -o '"tag_name": *"[^"]*"' | cut -d'"' -f4)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
BASE="https://github.com/memetics19/pulse/releases/download/${VERSION}"

curl -fsSLo pulse-agent "${BASE}/pulse-agent_${OS}_${ARCH}"
curl -fsSLo SHA256SUMS "${BASE}/SHA256SUMS"
grep " pulse-agent_${OS}_${ARCH}\$" SHA256SUMS | sed "s|pulse-agent_${OS}_${ARCH}|pulse-agent|" | shasum -a 256 -c -
chmod +x pulse-agent && sudo mv pulse-agent /usr/local/bin/
```

Run it as root so the `kernel` section can read the ring buffer; without that
the section degrades to a permission error. On a Proxmox node, install it on the
host rather than inside a guest: a hung VM cannot report on itself, so the
host-side view is the only reliable one.

Pulse does not ship a systemd unit for the agent. Invoke it directly, or write
your own unit if you want it scheduled.

## Collecting a bundle

Print a bundle locally, without contacting Pulse:

```sh
pulse-agent --diagnose
```

This works even when Pulse itself is unreachable, which is when you are most
likely to need it.

The agent targets Linux. On macOS the `processes` section degrades, because
`ps --sort` is procps-specific, and `systemd` and `proxmox` are absent.

Collect and upload in one step by supplying the agent's credentials:

```sh
pulse-agent --diagnose --server https://status.example.com --token <agent-token>
```

The token is the one shown once when the agent was created. See
[Architecture](architecture.md) for how agents are registered.

## API

```
POST /api/ingest/diagnostics
Authorization: Bearer <agent-token>
```

```json
{
  "bundle": { "collected_at": "...", "sections": {} }
}
```

A bundle belongs to the agent that produced it, identified by the bearer token,
and carries no other association. Uploads are limited to one per agent every
five seconds; a faster caller receives `429` with `Retry-After`. The agent
refuses to send a bundle larger than 900 KiB, which would be rejected by the
1 MiB request cap anyway — the local copy is still printed. The server stores it verbatim, so collectors
can change without a server-side migration. The bundle must be a JSON object;
its section contents are not validated. Responds `204 No Content`.

Bundles fall under the same `retention_days` window as check results and are
removed by the retention worker. Lowering that setting therefore discards older
diagnostic evidence as well as older check history — see
[Data retention](monitors.md#data-retention).

### Reading bundles back

```
GET /api/agents/{agentID}/diagnostics?limit=5
Authorization: Bearer <api-key>
```

Returns an agent's recent bundles, newest first, as JSON. Requires the
`diagnostics:read` scope — bundles carry journal entries, container logs,
process names, and filesystem paths, so they need an explicit grant rather than
riding on `agents:read`. Reading a host's evidence is an admin action, not
something the agent's own ingest token can do.

`limit` defaults to 3 and is capped at 5. Bundles are large, so the ceiling is
deliberately low.
