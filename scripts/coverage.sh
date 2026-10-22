#!/usr/bin/env bash
# Run tests for a Go module with coverage, excluding machine-generated code
# (internal/generated, sqlc output) from the metric, print a per-package table,
# and fail if the total is below the threshold.
#
# Usage: scripts/coverage.sh <module-dir> [threshold]
#   e.g. scripts/coverage.sh api 90
set -euo pipefail

MODULE="${1:?usage: coverage.sh <module-dir> [threshold]}"
THRESHOLD="${2:-90}"
EXCLUDE='internal/generated/'

cd "$(dirname "$0")/../$MODULE"

profile="coverage.out"
filtered="coverage.nogen.out"

go test ./... -covermode=atomic -coverprofile="$profile" >/dev/null

# Drop generated code from the profile before computing the total.
grep -v "$EXCLUDE" "$profile" > "$filtered" || true

pkgtable="$(go test ./... -cover 2>/dev/null \
  | grep -E 'coverage:|no test files' \
  | grep -v "$EXCLUDE" \
  | sed -E "s#github.com/memetics19/pulse/$MODULE/##" || true)"

echo "== $MODULE per-package coverage (excluding $EXCLUDE) =="
echo "$pkgtable"
echo

total="$(go tool cover -func="$filtered" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')"
echo "== $MODULE total (excluding generated): ${total}% (threshold ${THRESHOLD}%) =="

# Browsable HTML report (uploaded as a CI artifact).
go tool cover -html="$filtered" -o "coverage.html" 2>/dev/null || true

# Shields.io endpoint JSON so a README badge can read the live number.
color=red
awk -v t="$total" 'BEGIN{exit !(t+0 >= 90)}' && color=brightgreen
awk -v t="$total" 'BEGIN{exit !(t+0 >= 80 && t+0 < 90)}' && color=green
awk -v t="$total" 'BEGIN{exit !(t+0 >= 70 && t+0 < 80)}' && color=yellow
printf '{"schemaVersion":1,"label":"%s coverage","message":"%s%%","color":"%s"}\n' \
  "$MODULE" "$total" "$color" > "coverage-badge.json"

# GitHub Actions job summary.
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### \`$MODULE\` coverage: ${total}% (gate ${THRESHOLD}%, excluding generated)"
    echo '```'
    echo "$pkgtable"
    echo '```'
  } >> "$GITHUB_STEP_SUMMARY"
fi

# Numeric comparison with one decimal of precision.
if awk -v t="$total" -v th="$THRESHOLD" 'BEGIN{exit !(t+0 < th+0)}'; then
  echo "FAIL: coverage ${total}% is below ${THRESHOLD}%"
  exit 1
fi
echo "PASS"
