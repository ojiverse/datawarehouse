#!/usr/bin/env bash
# Runs lint + tests, then the #36 closed loop with the environment already
# exported by the caller (see spike/local/env.sh or the R2 variables in
# cmd/dwh-spike/main.go). Usage: scripts/spike.sh <check|loop|cleanup>
set -euxo pipefail

readonly repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly mode="${1:-check}"

check() {
  local unformatted
  unformatted="$(gofmt -l "${repo_root}/cmd" "${repo_root}/internal")"
  if [[ -n "${unformatted}" ]]; then
    echo "unformatted files: ${unformatted}" >&2
    exit 1
  fi
  go vet ./...
  go test ./...
}

cd "${repo_root}"
case "${mode}" in
  check)
    check
    ;;
  loop|cleanup)
    check
    go run ./cmd/dwh-spike "${mode}"
    ;;
  *)
    echo "usage: $0 <check|loop|cleanup>" >&2
    exit 2
    ;;
esac
