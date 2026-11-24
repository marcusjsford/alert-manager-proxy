#!/usr/bin/env bash
# Run the strict v1 contract and pass-through/inspection tests against one
# Alertmanager release. Used by CI for production, the selected upgrade target
# and the latest upstream release, and by the scheduled release workflow.
#
# Usage: test/release/check-version.sh v0.35.0
# Ports can be overridden with AM_COMPAT_PORT, PROXY_COMPAT_PORT and
# ADMIN_COMPAT_PORT when running several checks at once.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <alertmanager-version>" >&2
  exit 2
fi

version="${1#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid Alertmanager version: $1" >&2
  exit 2
fi
tag="v$version"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

workdir="$(mktemp -d "${TMPDIR:-/tmp}/amp-compat-$version.XXXXXX")"
am_port="${AM_COMPAT_PORT:-19093}"
proxy_port="${PROXY_COMPAT_PORT:-19095}"
admin_port="${ADMIN_COMPAT_PORT:-19096}"
am_pid=""
proxy_pid=""

cleanup() {
  if [[ -n "$proxy_pid" ]]; then
    kill "$proxy_pid" 2>/dev/null || true
    wait "$proxy_pid" 2>/dev/null || true
  fi
  if [[ -n "$am_pid" ]]; then
    kill "$am_pid" 2>/dev/null || true
    wait "$am_pid" 2>/dev/null || true
  fi
  rm -rf "$workdir"
}
trap cleanup EXIT

echo "== Alertmanager $tag compatibility =="

base="https://github.com/prometheus/alertmanager/releases/download/$tag"
tarball="alertmanager-$version.linux-amd64.tar.gz"
(
  cd "$workdir"
  curl -sSfL --retry 3 -O "$base/$tarball"
  curl -sSfL --retry 3 "$base/sha256sums.txt" | grep " $tarball\$" | sha256sum -c -
  tar xzf "$tarball"
)

"$workdir/alertmanager-$version.linux-amd64/alertmanager" \
  --config.file="$repo_root/test/contract/alertmanager.yml" \
  --storage.path="$workdir/am-data" \
  --web.listen-address="127.0.0.1:$am_port" \
  >"$workdir/alertmanager.log" 2>&1 &
am_pid=$!

for _ in $(seq 1 60); do
  curl -sf "http://127.0.0.1:$am_port/-/ready" >/dev/null && break
  sleep 1
done
if ! curl -sf "http://127.0.0.1:$am_port/-/ready" >/dev/null; then
  echo "Alertmanager $tag did not become ready" >&2
  tail -50 "$workdir/alertmanager.log" >&2
  exit 1
fi

go build -o "$workdir/alertmanager-proxy" ./cmd/alertmanager-proxy

AMP_UPSTREAM_URL="http://127.0.0.1:$am_port" \
  AMP_LISTEN_ADDRESS="127.0.0.1:$proxy_port" \
  AMP_ADMIN_LISTEN_ADDRESS="127.0.0.1:$admin_port" \
  "$workdir/alertmanager-proxy" >"$workdir/proxy.log" 2>&1 &
proxy_pid=$!

for _ in $(seq 1 30); do
  curl -sf "http://127.0.0.1:$admin_port/-/ready" >/dev/null && break
  sleep 1
done
if ! curl -sf "http://127.0.0.1:$admin_port/-/ready" >/dev/null; then
  echo "proxy did not become ready" >&2
  tail -50 "$workdir/proxy.log" >&2
  exit 1
fi

set +e
go run ./test/contract/cmd/contract verify -strict \
  -target "http://127.0.0.1:$proxy_port" \
  -fixtures test/contract/fixtures/v0.26.0 \
  -pending test/contract/pending.txt 2>&1 | tee "$workdir/contract.log"
contract_status=${PIPESTATUS[0]}

AMP_IT_UPSTREAM_URL="http://127.0.0.1:$am_port" \
  go test -tags integration -count=1 -v ./test/integration/ 2>&1 | tee "$workdir/integration.log"
integration_status=${PIPESTATUS[0]}
set -e

contract_result=FAIL
integration_result=FAIL
[[ "$contract_status" -eq 0 ]] && contract_result=PASS
[[ "$integration_status" -eq 0 ]] && integration_result=PASS

echo "REPORT alertmanager=$tag contract=$contract_result integration=$integration_result"

if [[ "$contract_status" -ne 0 || "$integration_status" -ne 0 ]]; then
  echo "--- Alertmanager log (last 50 lines)" >&2
  tail -50 "$workdir/alertmanager.log" >&2
  echo "--- Proxy log (last 50 lines)" >&2
  tail -50 "$workdir/proxy.log" >&2
  exit 1
fi
