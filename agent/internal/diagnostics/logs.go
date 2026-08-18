package diagnostics

// Log capture is deliberately narrow: logs are only pulled for units and
// containers already identified as failing. An OOM kill says a service died;
// the log says why. Nothing here takes a target from the server.

// logTailLines bounds how far back each log reaches.
const logTailLines = "200"

// maxLogTargets bounds how many failing units or containers get a log dump, so
// a host with many broken services still produces a sendable bundle.
const maxLogTargets = 5

// maxLogBytes bounds each captured log. The whole bundle is POSTed under the
// server's 1 MiB request cap, so one runaway log must not make it unsendable.
const maxLogBytes = 32 << 10

const logTruncationNotice = "\n[truncated: log exceeded capture limit]"

// truncateLog clamps a captured log to maxLogBytes, keeping the tail — the end
// of a log is where the failure is.
func truncateLog(out []byte) string {
	if len(out) <= maxLogBytes {
		return string(out)
	}
	return string(out[len(out)-maxLogBytes:]) + logTruncationNotice
}

// capTargets limits how many failing entities get logs collected.
func capTargets(names []string) []string {
	if len(names) > maxLogTargets {
		return names[:maxLogTargets]
	}
	return names
}
