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
| `disk` | `df -P` | Usage per mount, and which mounts are at capacity |
| `processes` | `ps` | The 15 busiest processes by CPU share |
| `docker` | `docker ps -a`, `docker logs` | All containers with state and exit status, which are not running, and recent output from those |
| `systemd` | `systemctl list-units --failed`, `journalctl` | Units in the failed state, and the recent journal for each |
| `proxmox` | `qm list` | Guest VMs and their status, and which are not running |

Every collector is read-only. The agent runs a fixed set of commands and never
executes anything supplied by the server.

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

Mounts that read 100% by design are listed but never reported as full: `devfs`,
`devtmpfs`, `udev`, `proc`, `sysfs`, `efivarfs`, and read-only image mounts on
`/dev/loop*` such as snap packages.

`tmpfs` and `overlay` are **not** suppressed. A full `tmpfs` is real
memory-backed exhaustion, and a full `overlay` is a container's writable layer
filling up — both are actionable.

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
  "incident_id": 7,
  "bundle": { "collected_at": "...", "sections": {} }
}
```

`incident_id` is optional; omit it for an on-demand bundle that belongs to no
incident. The server stores the bundle verbatim, so collectors can change
without a server-side migration. Responds `204 No Content`.
