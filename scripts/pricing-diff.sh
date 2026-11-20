#!/bin/sh
# Compares the prices of every pricing/<region>.json in the working tree with
# the same file at a git ref, and prints a Markdown summary per region.
# Only the "prices" map counts: a refresh that changes as_of, sha256 or the
# offer file versions and keeps every price is not a change.
# Usage: scripts/pricing-diff.sh [ref]   (default HEAD)
# Exit status, as diff(1): 0 no price changed, 1 prices changed, 2 trouble.
# Used by .github/workflows/refresh-pricing.yml.
set -eu

ref="${1:-HEAD}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
changed=0

for f in pricing/*.json; do
  region="$(basename "$f" .json)"
  if git cat-file -e "$ref:$f" 2>/dev/null; then
    git show "$ref:$f" > "$work/old.json" || exit 2
  else
    echo '{"prices": {}}' > "$work/old.json"
  fi
  jq -S .prices "$work/old.json" > "$work/old-prices.json" || exit 2
  jq -S .prices "$f" > "$work/new-prices.json" || exit 2
  if cmp -s "$work/old-prices.json" "$work/new-prices.json"; then
    echo "- \`$region\`: no price changed (as of $(jq -r .as_of "$f"))"
    continue
  fi
  changed=1
  jq -n -r --arg region "$region" --slurpfile old "$work/old.json" --slurpfile new "$f" '
    ($old[0].prices) as $o | ($new[0].prices) as $p |
    [$p | keys[] | select($o[.] == null)] as $added |
    [$o | keys[] | select($p[.] == null)] as $removed |
    [$p | keys[] | select($o[.] != null and $o[.] != $p[.])] as $changed |
    "- `\($region)`: \($added | length) prices added, \($removed | length) removed, \($changed | length) changed (as of \($new[0].as_of), was \($old[0].as_of // "absent"))",
    ($changed[:20][] | "  - changed `\(.)`: \($o[.].usd) → \($p[.].usd)"),
    ($removed[:20][] | "  - removed `\(.)`"),
    ($added[:20][] | "  - added `\(.)`: \($p[.].usd)"),
    (if ($added | length) > 20 or ($removed | length) > 20 or ($changed | length) > 20 then "  - (lists cut at 20 keys each)" else empty end)
  ' || exit 2
done
exit "$changed"
