# Integration: PR cost gate (GitHub Actions)

> Example job for a Terraform repository (first consumer: `leosavio/foundation-org-leo-plan`,
> not applied there yet). It follows supply-chain rules: actions pinned to a commit SHA with the
> tag in a comment, minimal `permissions:` per job, `persist-credentials: false`, the binary
> verified by checksum and signature, no `trivy-action`/`setup-trivy`. SHAs checked on 2026-11-03.

## Flow

```text
terraform plan -out=tfplan → terraform show -json tfplan > plan.json   (plan job, read-only OIDC)
          └─ artifact plan.json
costgate diff --plan plan.json --usage costgate-usage.json \
              --max-monthly-delta "$COST_DELTA_CEILING_USD" --markdown-out cost.md   (cost job)
          ├─ one PR comment (marker <!-- costgate -->), updated on every push
          └─ exit 0 passes · 1 or 2 fails unless the PR has the cost-approved label · 3 always fails
```

| Exit | Meaning | Without `cost-approved` | With `cost-approved` |
|---|---|---|---|
| 0 | within the ceiling, everything priced | pass | pass |
| 1 | delta (or total) above the ceiling | **fail** | pass |
| 2 | created/changed resource without a known price | **fail** | pass |
| 3 | usage or input error (invalid plan, invalid premises) | **fail** | **fail** |

Repository variable: `vars.COST_DELTA_CEILING_USD` (e.g. `10`). Premises versioned per stack in
`stacks/<stack>/costgate-usage.json` (format in the costgate `README.md`).

## Workflow

```yaml
name: plan

on:
  pull_request:
    types: [opened, synchronize, reopened, labeled, unlabeled]

permissions:
  contents: read

jobs:
  plan:
    runs-on: ubuntu-24.04
    permissions:
      contents: read
      id-token: write # OIDC: ci-entry-plan -> deploy-<stack>-plan (ADR-008)
    env:
      STACK_DIR: stacks/org-services
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false

      - uses: hashicorp/setup-terraform@dfe3c3f87815947d99a8997f908cb6525fc44e9e # v4.0.1
        with:
          terraform_wrapper: false

      # (assume the plan role through OIDC here)

      - name: plan
        working-directory: ${{ env.STACK_DIR }}
        run: |
          terraform init -input=false
          terraform plan -input=false -lock=false -out=tfplan
          terraform show -json tfplan > plan.json

      - uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2
        with:
          name: plan-json
          path: ${{ env.STACK_DIR }}/plan.json
          retention-days: 7

  cost:
    needs: plan
    runs-on: ubuntu-24.04
    permissions:
      contents: read
      pull-requests: write # only job that comments on the PR
    env:
      COSTGATE_VERSION: 0.2.0
      STACK_DIR: stacks/org-services
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false

      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2
        with:
          name: plan-json

      - uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2
        with:
          cosign-release: v3.1.4

      - name: install costgate (checksum + cosign keyless)
        run: |
          set -eu
          v="$COSTGATE_VERSION"
          base="https://github.com/leosavio/costgate/releases/download/v${v}"
          tarball="costgate_${v}_linux_amd64.tar.gz"
          curl -fsSLO "${base}/${tarball}"
          curl -fsSLO "${base}/checksums.txt"
          curl -fsSLO "${base}/checksums.txt.sigstore.json"
          cosign verify-blob --bundle checksums.txt.sigstore.json \
            --certificate-identity "https://github.com/leosavio/costgate/.github/workflows/release.yml@refs/tags/v${v}" \
            --certificate-oidc-issuer https://token.actions.githubusercontent.com \
            checksums.txt
          grep " ${tarball}\$" checksums.txt | sha256sum -c -
          tar -xzf "$tarball" costgate
          ./costgate version

      - name: costgate diff
        id: costgate
        env:
          CEILING: ${{ vars.COST_DELTA_CEILING_USD }}
        run: |
          set +e
          ./costgate diff --plan plan.json \
            --usage "${STACK_DIR}/costgate-usage.json" \
            --max-monthly-delta "${CEILING:?set vars.COST_DELTA_CEILING_USD}" \
            --markdown-out cost.md --json-out cost.json > /dev/null
          echo "exit=$?" >> "$GITHUB_OUTPUT"

      - name: comment on the PR (single, updated)
        if: always() && steps.costgate.outputs.exit != '3'
        env:
          GH_TOKEN: ${{ github.token }}
          PR: ${{ github.event.pull_request.number }}
        run: |
          set -eu
          id="$(gh api --paginate "repos/${GITHUB_REPOSITORY}/issues/${PR}/comments" \
            -q '.[] | select(.user.login == "github-actions[bot]" and (.body | startswith("<!-- costgate -->"))) | .id' | head -n1)"
          if [ -n "$id" ]; then
            gh api -X PATCH "repos/${GITHUB_REPOSITORY}/issues/comments/${id}" -F body=@cost.md > /dev/null
          else
            gh pr comment "$PR" --repo "$GITHUB_REPOSITORY" --body-file cost.md
          fi

      - name: gate
        env:
          EXIT: ${{ steps.costgate.outputs.exit }}
          APPROVED: ${{ contains(github.event.pull_request.labels.*.name, 'cost-approved') }}
        run: |
          case "$EXIT" in
            0) echo "cost within ceiling" ;;
            1|2)
              if [ "$APPROVED" = "true" ]; then
                echo "exit $EXIT accepted by label cost-approved"
              else
                echo "::error::costgate exit $EXIT (1 = above ceiling, 2 = unpriced resource). Add the label cost-approved to accept."
                exit 1
              fi ;;
            *) echo "::error::costgate failed (exit $EXIT): usage or input error"; exit 1 ;;
          esac
```

## Notes

| Topic | Rule |
|---|---|
| Several stacks | one `cost` job per stack (matrix). The marker is fixed; to keep separate comments, prefix the file (`sed -i '1s/costgate/costgate:<stack>/' cost.md`) and adjust the `startswith` filter |
| `cost-approved` label | whoever can apply labels decides; protect it with branch rules/CODEOWNERS for third-party PRs. The `labeled` event re-runs the workflow |
| Fork PRs | get neither `id-token` nor `pull-requests: write`; do not accept fork PRs for stacks |
| OpenTofu | `tofu show -json` emits the same format (`format_version` 1.x) |
| Upgrading costgate | change `COSTGATE_VERSION`; the verification command stays the same |
| Comment size | GitHub caps comments at 65,536 characters; a plan with hundreds of changed resources may exceed it (not verified). Since 0.2.0 the collapsed "Billing keys" block adds one row per cost line |
| Bill keys | `--json-lines cost.jsonl` writes one document per cost line with `sku`, `usage_type`, `rate_code` and `billing_rule`, for Athena or `jq`; the JSON report has the same fields |
