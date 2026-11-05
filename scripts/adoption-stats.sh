#!/bin/sh
# Adoption metrics of a GitHub-hosted CLI, for its maintainer. Needs gh (logged
# in with admin access to the traffic repositories) and jq. Nothing here runs
# in the costgate binary: costgate itself never sends data.
#
# Usage:
#   scripts/adoption-stats.sh collect --out FILE [--repo OWNER/NAME]...
#       [--traffic OWNER/NAME]... [--own-ci OWNER/NAME:WORKFLOW_FILE]...
#   scripts/adoption-stats.sh report --in FILE
#
# collect appends a snapshot to FILE (JSON Lines, schema_version 1) and keeps
# one line per key, the newest one, sorted, so the file can live in git:
#   repo          stars, forks and watchers of --repo, per day
#   downloads     cumulative download_count of each release asset of --repo
#   traffic       views and clones per day of --traffic (GitHub keeps 14 days)
#   traffic_window views and clones over the 14-day window, with uniques
#   traffic_top   top referrers and paths over the 14-day window
#   own_ci        jobs of our own workflows whose install step ran (so they
#                 downloaded a release asset or cloned the tap), per day, kind
#                 (first word of the job name: brew, tarball, go, cost,
#                 release) and platform (from the runner labels)
# report prints a Markdown summary: own_ci is subtracted from the raw counts,
# so the remainder estimates downloads by others (still an upper bound: other
# people's CI counts too).
set -eu

SCHEMA_VERSION=1

die() { echo "adoption-stats: $*" >&2; exit 2; }

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }

collect() {
  out="" repos="" traffic="" own=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --out) out="${2:?--out FILE}"; shift 2 ;;
      --repo) repos="$repos ${2:?--repo OWNER/NAME}"; shift 2 ;;
      --traffic) traffic="$traffic ${2:?--traffic OWNER/NAME}"; shift 2 ;;
      --own-ci) own="$own ${2:?--own-ci OWNER/NAME:WORKFLOW}"; shift 2 ;;
      *) die "collect: unknown argument $1" ;;
    esac
  done
  [ -n "$out" ] || die "collect: --out is required"
  at="$(now_utc)"
  today="${at%%T*}"
  new="$(mktemp)"
  trap 'rm -f "$new" "$new.merged"' EXIT

  for r in $repos; do
    gh api "repos/$r" | jq -c --arg r "$r" --arg d "$today" --arg at "$at" --argjson v "$SCHEMA_VERSION" \
      '{schema_version: $v, kind: "repo", repo: $r, date: $d, stars: .stargazers_count,
        forks: .forks_count, watchers: .subscribers_count, collected_at: $at}' >> "$new"
    gh api --paginate "repos/$r/releases?per_page=100" | jq -c --arg r "$r" --arg d "$today" --arg at "$at" --argjson v "$SCHEMA_VERSION" \
      '.[] | .tag_name as $t | .assets[] |
       {schema_version: $v, kind: "downloads", repo: $r, date: $d, tag: $t, asset: .name,
        count: .download_count, collected_at: $at}' >> "$new"
  done

  for r in $traffic; do
    for m in views clones; do
      gh api "repos/$r/traffic/$m" | jq -c --arg r "$r" --arg m "$m" --arg d "$today" --arg at "$at" --argjson v "$SCHEMA_VERSION" \
        '({schema_version: $v, kind: "traffic_window", repo: $r, metric: $m, window_end: $d,
           count: .count, uniques: .uniques, collected_at: $at}),
         (.[$m][] | {schema_version: $v, kind: "traffic", repo: $r, metric: $m,
           date: .timestamp[0:10], count: .count, uniques: .uniques, collected_at: $at})' >> "$new"
    done
    gh api "repos/$r/traffic/popular/referrers" | jq -c --arg r "$r" --arg d "$today" --arg at "$at" --argjson v "$SCHEMA_VERSION" \
      '.[] | {schema_version: $v, kind: "traffic_top", repo: $r, metric: "referrer", window_end: $d,
              key: .referrer, count: .count, uniques: .uniques, collected_at: $at}' >> "$new"
    gh api "repos/$r/traffic/popular/paths" | jq -c --arg r "$r" --arg d "$today" --arg at "$at" --argjson v "$SCHEMA_VERSION" \
      '.[] | {schema_version: $v, kind: "traffic_top", repo: $r, metric: "path", window_end: $d,
              key: .path, count: .count, uniques: .uniques, collected_at: $at}' >> "$new"
  done

  for spec in $own; do
    repo="${spec%%:*}"
    wf="${spec#*:}"
    [ "$repo" != "$spec" ] && [ -n "$wf" ] || die "--own-ci wants OWNER/NAME:WORKFLOW_FILE, got $spec"
    # Rescan whole days from two days before the newest day already collected,
    # so a day is always recounted in full; the first run scans all history.
    since=""
    if [ -f "$out" ]; then
      last="$(jq -r --arg w "$spec" 'select(.kind == "own_ci" and .workflow == $w) | .date' "$out" | sort | tail -n 1)"
      [ -n "$last" ] && since="$(date -u -j -v-2d -f %Y-%m-%d "$last" +%Y-%m-%d 2>/dev/null || date -u -d "$last - 2 days" +%Y-%m-%d)"
    fi
    query="per_page=100"
    [ -n "$since" ] && query="$query&created=%3E%3D$since"
    gh api --paginate "repos/$repo/actions/workflows/$wf/runs?$query" --jq '.workflow_runs[].id' |
      while read -r id; do
        gh api --paginate "repos/$repo/actions/runs/$id/jobs?filter=all&per_page=100" --jq '.jobs[]'
      done |
      jq -c -s --arg w "$spec" --arg at "$at" --argjson v "$SCHEMA_VERSION" '
        def platform:
          if any(.labels[]; . == "ubuntu-24.04-arm" or . == "ubuntu-22.04-arm") then "linux_arm64"
          elif any(.labels[]; startswith("ubuntu")) then "linux_amd64"
          elif any(.labels[]; . == "macos-15-intel" or . == "macos-13") then "darwin_amd64"
          elif any(.labels[]; startswith("macos")) then "darwin_arm64"
          else "other" end;
        # A job counts when its install step ran (it downloaded an asset or
        # cloned the tap); the release job clones the tap to update the formula.
        map(select(any(.steps[]?; (.name | test("^(install|update homebrew tap)"; "i")) and (.conclusion == "success" or .conclusion == "failure")))
            | {date: .started_at[0:10], job_kind: (.name | split(" ")[0] | ascii_downcase), platform: platform}
            | select(.job_kind == "brew" or .job_kind == "tarball" or .job_kind == "go" or .job_kind == "cost" or .job_kind == "release"))
        | group_by([.date, .job_kind, .platform])
        | .[] | {schema_version: $v, kind: "own_ci", workflow: $w, date: .[0].date,
                 job_kind: .[0].job_kind, platform: .[0].platform, count: length, collected_at: $at}' >> "$new"
  done

  touch "$out"
  other="$(jq -s --argjson v "$SCHEMA_VERSION" 'map(select(.schema_version != $v)) | length' "$out")"
  [ "$other" = 0 ] || die "$out has $other lines with a schema_version other than $SCHEMA_VERSION; refusing to merge"
  # Merge: newest line per key wins; stable sort for readable git diffs.
  jq -c -s '
    def key: [.kind, (.repo // .workflow), (.metric // ""), (.date // .window_end),
              (.tag // ""), (.asset // .key // .job_kind // ""), (.platform // "")];
    group_by(key) | map(max_by(.collected_at)) | sort_by(key) | .[]' \
    "$out" "$new" > "$new.merged"
  mv "$new.merged" "$out"
  echo "adoption-stats: $(wc -l < "$out" | tr -d ' ') lines in $out (collected $at)"
}

report() {
  in=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --in) in="${2:?--in FILE}"; shift 2 ;;
      *) die "report: unknown argument $1" ;;
    esac
  done
  [ -f "$in" ] || die "report: $in not found"
  jq -r -s '
    def newest(k): map(select(.kind == k)) | (map(.collected_at) | max) as $m | map(select(.collected_at == $m));
    def plat: capture("_(?<p>(darwin|linux)_(amd64|arm64))\\.tar\\.gz$").p;
    (map(select(.schema_version > 1)) | length) as $newer
    | if $newer > 0 then error("\($newer) lines have a schema_version above 1; upgrade this script") else . end
    | . as $all
    | (newest("downloads")) as $dl
    | (map(select(.kind == "own_ci"))) as $ci
    | (newest("traffic_window")) as $tw
    | (newest("repo")) as $repo
    | (newest("traffic_top")) as $top
    | def own(p; kinds): [$ci[] | select(.platform == p) | select(.job_kind as $k | any(kinds[]; . == $k)) | .count] | add // 0;
      "# Adoption, as of \($dl[0].collected_at // "never")",
      "",
      "| Repository | Stars | Forks | Watchers |", "|---|---:|---:|---:|",
      ($repo[] | "| \(.repo) | \(.stars) | \(.forks) | \(.watchers) |"),
      "",
      "## Traffic, last 14 days",
      "",
      "| Repository | Metric | Count | Uniques |", "|---|---|---:|---:|",
      ($tw | sort_by(.repo, .metric)[] | "| \(.repo) | \(.metric) | \(.count) | \(.uniques) |"),
      "",
      "Top referrers and paths:",
      "",
      (if ($top | length) == 0 then "- none yet" else ($top | sort_by(.repo, .metric, -.count)[] | "- \(.repo) \(.metric) `\(.key)`: \(.count) (\(.uniques) uniques)") end),
      "",
      "## Release downloads, all versions",
      "",
      "Own CI = jobs of our workflows that downloaded the asset (brew and tarball jobs of install-smoke, cost jobs of the consumer). Manual downloads by the maintainer are not subtracted.",
      "",
      "| Platform | Tarball downloads | Own CI jobs | Others (upper bound) |", "|---|---:|---:|---:|",
      ([$dl[] | select(.asset | test("\\.tar\\.gz$")) | {p: (.asset | plat), c: .count}]
       | group_by(.p) | .[] | (.[0].p) as $p | (map(.c) | add) as $t | own($p; ["brew", "tarball", "cost"]) as $o
       | "| \($p) | \($t) | \($o) | \($t - $o) |"),
      ([$dl[] | select(.asset == "checksums.txt") | .count] | add // 0) as $ck
      | ([$ci[] | select(.job_kind == "tarball" or .job_kind == "cost") | .count] | add // 0) as $ock
      | "| checksums.txt (verified installs) | \($ck) | \($ock) | \($ck - $ock) |",
      "",
      "## Homebrew tap, last 14 days",
      "",
      ((now - 14*86400) | strftime("%Y-%m-%d")) as $from
      | ([$ci[] | select((.job_kind == "brew" or .job_kind == "release") and .date >= $from) | .count] | add // 0) as $oc
      | ([$tw[] | select(.metric == "clones" and (.repo | endswith("/homebrew-tap")))][0]) as $tc
      | "Tap clones: \($tc.count // 0) (\($tc.uniques // 0) uniques). Own CI jobs that clone the tap (brew installs and formula updates): \($oc). A clone is not an install: calibrate before reading it as one."
  ' "$in"
}

cmd="${1:-}"
[ $# -gt 0 ] && shift
case "$cmd" in
  collect) collect "$@" ;;
  report) report "$@" ;;
  *) die "usage: $0 collect --out FILE [...] | report --in FILE" ;;
esac
