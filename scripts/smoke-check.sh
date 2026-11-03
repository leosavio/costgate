#!/bin/sh
# Checks an installed costgate binary against the golden of the release tag.
# Usage: scripts/smoke-check.sh <path to costgate> <version without v>
# Run from a checkout of the same tag: the expected output comes from
# testdata/cases/baseline. Used by .github/workflows/install-smoke.yml.
set -eu

bin="${1:?path to costgate}"
version="${2:?version}"
case_dir="testdata/cases/baseline"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# 1. version
got="$("$bin" version)"
if [ "$got" != "costgate ${version}" ]; then
  echo "version: got '$got', want 'costgate ${version}'" >&2
  exit 1
fi
echo "ok version: $got"

# 2. embedded snapshot
"$bin" pricing verify > "$work/verify.txt"
head -n 1 "$work/verify.txt"
grep -q '^ok us-east-1: ' "$work/verify.txt" || { echo "pricing verify: unexpected output" >&2; exit 1; }

# 3. baseline case: exit code and Markdown equal to the golden. The golden is
# rendered by the tests with version "test" and the fixture snapshot, so the
# footer line (version, snapshot date and hash) is the only line excluded.
want_exit="$(cat "$case_dir/want_exit")"
set +e
# shellcheck disable=SC2046 # args.txt holds whitespace-separated flags
"$bin" diff --plan "$case_dir/plan.json" --usage "$case_dir/usage.json" $(cat "$case_dir/args.txt") > "$work/got.md"
code=$?
set -e
if [ "$code" != "$want_exit" ]; then
  echo "baseline: exit $code, want $want_exit" >&2
  cat "$work/got.md" >&2
  exit 1
fi
grep -v '^<sub>costgate ' "$case_dir/want.md" > "$work/want.body"
grep -v '^<sub>costgate ' "$work/got.md" > "$work/got.body"
if ! diff -u "$work/want.body" "$work/got.body"; then
  echo "baseline: Markdown differs from $case_dir/want.md" >&2
  exit 1
fi
echo "ok baseline: exit $code, $(grep -m 1 '^### costgate:' "$work/got.md")"
