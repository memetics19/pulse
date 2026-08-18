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
| `docker` | `docker ps -a` | All containers with state and exit status, and which are not running |
| `systemd` | `systemctl list-units --failed` | Units in the failed state |
| `proxmox` | `qm list` | Guest VMs and their status, and which are not running |

Every collector is read-only. The agent runs a fixed set of commands and never
executes anything supplied by the server.

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

Mounts on pseudo filesystems (`tmpfs`, `devfs`, `udev`, and similar) are listed
but never reported as full. They permanently report 100% capacity because they
have no backing store.

## Collecting a bundle

Print a bundle locally, without contacting Pulse:

```sh
pulse-agent --diagnose
```

This works even when Pulse itself is unreachable, which is when you are most
likely to need it.

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
