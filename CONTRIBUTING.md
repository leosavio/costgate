# Contributing

Pull requests and issues are welcome. Short rules:

| Topic | Rule |
|---|---|
| Dependencies | standard library only. A new dependency needs a justification in the PR |
| Determinism | no clock, no unsorted map iteration, no `float64` for money; use `internal/money` |
| New resource type | formula in `internal/estimate/resources.go`, SKU rule in `internal/catalog/rules.go`, a row in `README.md` (and `README.pt-BR.md`) and a case in `testdata/cases/` |
| Zero-cost type | entry in `zeroCostTypes` with an AWS pricing source in the PR |
| Golden files | `go test ./internal/cli -update` and review the diff; CI runs on amd64 against goldens generated on arm64 |
| Checks | `gofmt -l .`, `go vet ./...`, `staticcheck ./...`, `go test -race ./...` clean |
| Workflows | actions pinned to a full commit SHA with the tag in a comment; minimal `permissions:`; `persist-credentials: false` |
| Commits | English, imperative mood (`feat:`, `fix:`, `docs:`, `chore:`) |
| Docs | `README.md` is the official version; keep `README.pt-BR.md` in sync in the same PR when possible |

Architecture decisions live in `docs/adr/` (written in Portuguese). An accepted ADR is not
rewritten; a change takes a new ADR.

## Release

Push a `vX.Y.Z` tag. `release.yml` tests, builds the four archives, writes `checksums.txt`,
signs it with cosign (keyless), publishes the GitHub release and pushes the generated formula
to [`leosavio/homebrew-tap`](https://github.com/leosavio/homebrew-tap) with a deploy key
(`HOMEBREW_TAP_DEPLOY_KEY`) that can write only to that repository. `go install` picks the tag
up from the Go module proxy with no extra step.
