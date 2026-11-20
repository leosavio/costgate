#!/usr/bin/env bash
# refresh-all: rebuilds pricing/<region>.json.gz for every region costgate supports, a few regions
# at a time. us-east-1 runs with --strict (it has every product, so a rule without a price there means
# the offer format changed). A region whose prices did not change keeps its file, so its embedded
# bytes and its as_of stay as they were.
#
#   bash scripts/refresh-all.sh                  every supported region
#   bash scripts/refresh-all.sh eu-west-1 ...    only these regions
#
#   COSTGATE   costgate binary (default: go run ./cmd/costgate, built once into a temp dir)
#   JOBS       regions refreshed at the same time (default 3)
#   OUT        output directory (default pricing)
set -euo pipefail

cd "$(dirname "$0")/.."
jobs=${JOBS:-3}
out=${OUT:-pricing}
tmp=$(mktemp -d "${TMPDIR:-/tmp}/costgate-refresh.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

bin=${COSTGATE:-}
if [ -z "$bin" ]; then
  bin=$tmp/costgate
  go build -o "$bin" ./cmd/costgate
fi

if [ $# -gt 0 ]; then
  regions=("$@")
else
  # shellcheck disable=SC2207 # region codes have no spaces; bash 3.2 (macOS) has no mapfile
  regions=($("$bin" pricing regions | cut -f1))
fi

# prices_of <file>: the prices map of a snapshot, canonical, for comparison.
prices_of() { gzip -dc "$1" | jq -S -c '.prices'; }

refresh_one() {
  local r=$1 new=$tmp/$1.json.gz strict=()
  [ "$r" = us-east-1 ] && strict=(--strict)
  if ! "$bin" pricing refresh --region "$r" --out "$new" "${strict[@]}" >"$tmp/$r.log" 2>&1; then
    echo "FAIL  $r"
    sed 's/^/      /' "$tmp/$r.log"
    return 1
  fi
  local dst=$out/$r.json.gz
  if [ -f "$dst" ] && [ "$(prices_of "$dst")" = "$(prices_of "$new")" ]; then
    echo "same  $r"
  else
    mv "$new" "$dst"
    echo "new   $r  $(grep '^wrote' "$tmp/$r.log" | sed 's/^wrote [^:]*: //')"
  fi
  grep '^costgate:' "$tmp/$r.log" | sed 's/^/      /' || true
}
export -f refresh_one prices_of
export bin tmp out

printf '%s\n' "${regions[@]}" | xargs -P "$jobs" -I{} bash -c 'refresh_one "$@"' _ {}
