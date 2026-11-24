#!/usr/bin/env bash
# Print the latest upstream Alertmanager release without the leading "v".
#
# Uses the GitHub "latest release" redirect rather than the API so scheduled
# CI does not need a token or hit the API rate limit. Override with
# AM_LATEST_VERSION for an offline or pinned run.
set -euo pipefail

if [[ -n "${AM_LATEST_VERSION:-}" ]]; then
  version="${AM_LATEST_VERSION#v}"
else
  release_url="$(
    curl -sSIL -o /dev/null -w '%{url_effective}' \
      https://github.com/prometheus/alertmanager/releases/latest
  )"
  tag="${release_url##*/}"
  if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
    echo "could not determine latest Alertmanager release from $release_url" >&2
    exit 1
  fi
  version="${tag#v}"
fi

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid Alertmanager version: $version" >&2
  exit 1
fi

printf '%s\n' "$version"
