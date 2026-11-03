# Integração: gate de custo no PR (GitHub Actions)

> Job de exemplo para o `leosavio/foundation-org-leo-plan`. **Não aplicado** no foundation ainda.
> Segue `foundation/docs/tooling/supply-chain.md`: actions por SHA com o tag em comentário,
> `permissions:` mínimas por job, `persist-credentials: false`, binário verificado por checksum e
> assinatura, sem `trivy-action`/`setup-trivy`. SHAs conferidos em 2026-11-03 e iguais aos de
> `foundation/docs/tooling/versions.md`.

## Fluxo

```text
terraform plan -out=tfplan → terraform show -json tfplan > plan.json   (job plan, OIDC de leitura)
          └─ artifact plan.json
costgate diff --plan plan.json --usage costgate-usage.json \
              --max-monthly-delta "$COST_DELTA_CEILING_USD" --markdown-out cost.md   (job cost)
          ├─ comentário único no PR (marcador <!-- costgate -->), atualizado a cada push
          └─ exit 0 passa · 1 ou 2 falha, salvo label cost-approved · 3 falha sempre
```

| Exit | Significado | Sem `cost-approved` | Com `cost-approved` |
|---|---|---|---|
| 0 | dentro do teto, tudo precificado | passa | passa |
| 1 | delta (ou total) acima do teto | **falha** | passa |
| 2 | recurso criado/alterado sem preço conhecido | **falha** | passa |
| 3 | erro de uso ou de entrada (plan inválido, premissa inválida) | **falha** | **falha** |

Variável do repositório: `vars.COST_DELTA_CEILING_USD` (ex.: `10`). Premissas versionadas por
stack em `stacks/<stack>/costgate-usage.json` (formato no `README.md` do costgate).

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

      # (assume role por OIDC aqui, como no workflow de plan do foundation)

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
      pull-requests: write # único job que comenta no PR
    env:
      COSTGATE_VERSION: 0.1.0
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

## Notas

| Ponto | Regra |
|---|---|
| Várias stacks | um job `cost` por stack (matrix). O marcador é fixo; para separar comentários, prefixe o arquivo (`sed -i '1s/costgate/costgate:<stack>/' cost.md`) e ajuste o `startswith` |
| Label `cost-approved` | quem pode aplicar label decide; proteja com regra de branch/CODEOWNERS se o PR for de terceiros. O evento `labeled` reroda o workflow |
| PR de fork | não recebe `id-token` nem `pull-requests: write`; o foundation não aceita PR de fork para stacks |
| Plan e OpenTofu | `tofu show -json` gera o mesmo formato (`format_version` 1.x) |
| Atualizar o costgate | trocar `COSTGATE_VERSION`; o comando de verificação não muda |
| Tamanho do comentário | GitHub limita a 65 536 caracteres; plan com centenas de recursos alterados pode exceder (não verificado) |
