#!/bin/sh
# Compares the prices of every pricing/<region>.json.gz in the working tree
# with the same file at a git ref, and prints a Markdown summary of the
# regions whose prices changed (the others on one line).
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
same=""

for f in pricing/*.json.gz; do
  region="$(basename "$f" .json.gz)"
  if git cat-file -e "$ref:$f" 2>/dev/null; then
    git show "$ref:$f" | gzip -dc > "$work/old.json" || exit 2
  else
    echo '{"prices": {}}' > "$work/old.json"
  fi
  gzip -dc "$f" > "$work/new.json" || exit 2
  jq -S .prices "$work/old.json" > "$work/old-prices.json" || exit 2
  jq -S .prices "$work/new.json" > "$work/new-prices.json" || exit 2
  if cmp -s "$work/old-prices.json" "$work/new-prices.json"; then
    same="$same \`$region\`"
    continue
  fi
  changed=1
  jq -n -r --arg region "$region" --slurpfile old "$work/old.json" --slurpfile new "$work/new.json" '
    ($old[0].prices) as $o | ($new[0].prices) as $p |
    [$p | keys[] | select($o[.] == null)] as $added |
    [$o | keys[] | select($p[.] == null)] as $removed |
    [$p | keys[] | select($o[.] != null and $o[.] != $p[.])] as $changed |
    "- `\($region)`: \($added | length) prices added, \($removed | length) removed, \($changed | length) changed (as of \($new[0].as_of), was \($old[0].as_of // "absent"))",
    ($changed[:10][] | "  - changed `\(.)`: \($o[.].usd) → \($p[.].usd)"),
    ($removed[:10][] | "  - removed `\(.)`"),
    ($added[:10][] | "  - added `\(.)`: \($p[.].usd)"),
    (if ($added | length) > 10 or ($removed | length) > 10 or ($changed | length) > 10 then "  - (lists cut at 10 keys each)" else empty end)
  ' || exit 2
done
if [ -n "$same" ]; then
  echo "- no price changed in:$same"
fi
exit "$changed"
