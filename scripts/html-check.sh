#!/usr/bin/env bash
# html-check: loads every golden HTML report (testdata/cases/*/want.html) in headless Chrome for
# several URL-hash states and fails unless each page reports data-status="ok" on <body> (the page
# sets "error: ..." on a script error or a Content-Security-Policy violation). It also checks that
# the default view shows every resource, that a filter with no match shows none, that an opened
# row renders its cost lines, that the page shows as many warnings and ignored resources as the JSON
# report, and that the escape case creates no <img> element.
#
#   bash scripts/html-check.sh                                       check the goldens
#   bash scripts/html-check.sh screenshot <out.png> [case] [hash]    1600x1000 screenshot of a golden
#                                                                    (default case: baseline)
#
#   CHROME   browser binary (default: Google Chrome on macOS, else google-chrome or chromium)
#
# Run `go test ./internal/cli` first: it proves the goldens match the current code.
set -euo pipefail

cd "$(dirname "$0")/.."

chrome=${CHROME:-}
if [ -z "$chrome" ]; then
  mac="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  if [ -x "$mac" ]; then
    chrome=$mac
  else
    for c in google-chrome chromium; do
      if command -v "$c" >/dev/null 2>&1; then chrome=$c; break; fi
    done
  fi
fi
if [ -z "$chrome" ]; then
  echo "html-check: Google Chrome or Chromium not found; set CHROME" >&2
  exit 1
fi

profile=$(mktemp -d "${TMPDIR:-/tmp}/costgate-html-check.XXXXXX")
trap 'rm -rf "$profile"' EXIT
flags=(--headless=new --disable-gpu --no-first-run --no-default-browser-check --user-data-dir="$profile"
  --window-size=1600,1000 --virtual-time-budget=4000)

if [ "${1:-}" = screenshot ]; then
  out=${2:?usage: html-check.sh screenshot <out.png> [case] [hash]}
  case $out in /*) ;; *) out=$PWD/$out ;; esac
  # Light color scheme whatever the OS setting, so the image does not depend on the machine.
  "$chrome" "${flags[@]}" --hide-scrollbars --blink-settings=preferredColorScheme=1 --screenshot="$out" \
    "file://$PWD/testdata/cases/${3:-baseline}/want.html#${4:-}" >/dev/null 2>&1
  echo "html-check: wrote $out"
  exit 0
fi

urlencode() {
  local s=$1 out="" c i h
  for ((i = 0; i < ${#s}; i++)); do
    c=${s:i:1}
    case $c in
      [a-zA-Z0-9._~-]) out+=$c ;;
      *) printf -v h '%%%02X' "'$c"; out+=$h ;;
    esac
  done
  printf '%s' "$out"
}

fail=0 total=0
# check <case> <hash> <expectation>: ok | all | none | open | noimg
check() {
  local page="$PWD/testdata/cases/$1/want.html" dom tag ok=1 why=""
  dom=$("$chrome" "${flags[@]}" --dump-dom "file://$page#$2" 2>/dev/null || true)
  tag=$(printf '%s' "$dom" | grep -o '<body[^>]*>' | head -1 || true)
  attr() { printf '%s' "$tag" | sed -n "s/.* $1=\"\\([^\"]*\\)\".*/\\1/p"; }
  if [[ $tag != *'data-status="ok"'* ]]; then ok=0; why="status"; fi
  case $3 in
    all) [ -n "$(attr data-rows)" ] && [ "$(attr data-rows)" = "$(attr data-total)" ] || { ok=0; why="rows $(attr data-rows) of $(attr data-total)"; } ;;
    none) [ "$(attr data-rows)" = 0 ] || { ok=0; why="rows $(attr data-rows), want 0"; } ;;
    open) [[ $dom == *'class="detail"'* ]] || { ok=0; why="no detail row"; } ;;
    noimg) [[ $dom != *'<img'* ]] || { ok=0; why="an <img> element was created"; } ;;
    counts)
      # Warnings and ignored resources shown = those of the JSON report.
      local w i
      w=$(grep -c '^      "code": ' "$PWD/testdata/cases/$1/want.json" || true)
      i=$(grep -c '^      "provider": ' "$PWD/testdata/cases/$1/want.json" || true)
      [ "$(attr data-warnings)" = "$w" ] && [ "$(attr data-ignored)" = "$i" ] ||
        { ok=0; why="warnings $(attr data-warnings) want $w, ignored $(attr data-ignored) want $i"; } ;;
  esac
  total=$((total + 1))
  if [ $ok = 1 ]; then
    echo "ok    $1 #$2  ${tag#<body }"
  else
    echo "FAIL  $1 #$2  $why  ${tag:-no <body> in the dump}"
    fail=$((fail + 1))
  fi
}

for dir in testdata/cases/*/; do
  name=$(basename "$dir")
  [ -f "$dir/want.html" ] || continue
  # The first resource address of the JSON report, unescaped, for the open-row state.
  addr=$(grep -m1 '^      "address": "' "$dir/want.json" | sed -e 's/^      "address": "//' -e 's/",$//' -e 's/\\"/"/g')
  check "$name" "" all
  check "$name" "" counts
  check "$name" "q=no-such-resource" none
  check "$name" "show=changed&metric=delta" ok
  check "$name" "show=unpriced&action=create" ok
  check "$name" "why=type&metric=delta" ok
  check "$name" "why=region&show=blocking" ok
  check "$name" "open=$(urlencode "$addr")&q=$(urlencode "$addr")" open
  if [ "$name" = escape ]; then
    check "$name" "" noimg
  fi
done

echo "html-check: $((total - fail))/$total pages ok"
[ "$fail" -eq 0 ]
