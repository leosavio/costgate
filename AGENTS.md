---
type: guide
summary: "Contrato de contexto do repo costgate para agentes: CLI Go (MIT) que projeta o custo mensal AWS de um terraform/tofu plan com snapshot da Price List Bulk API e gate fail-closed de PR."
updated: 2026-11-20
tags: [agents, aws, cost, terraform]
---

# AGENTS.md — costgate

> Contrato de contexto para agentes (LLM) e para o operador.
> Objetivo: sessão fria → produtiva em < 2 min.
> Atualize "Estado atual" ao encerrar qualquer sessão de trabalho relevante.
> Data plane transversal: `~/Tarscase/workspacePessoal/leosavio-org`
> (`knowledge/registry/repos.json`, `knowledge/intent/`, `guide-for-ai.md`).
> Frente no data plane: `leosavio-org/knowledge/work/costgate.md`.

## O que é

CLI em Go, MIT, repo público `leosavio/costgate`: lê `terraform show -json` de um plan, calcula o
custo mensal antes × depois por recurso com preços de um snapshot embutido da AWS Price List
Bulk API e devolve Markdown/JSON + exit code para gate de PR. Primeiro consumidor:
`leosavio/foundation-org-leo-plan` (substitui o Infracost previsto na ADR-011 dele).

## Stack e arquitetura

| Componente | Tecnologia | Nota |
|---|---|---|
| `cmd/costgate` | Go 1.27, só stdlib | binário único; `-X main.version` no release |
| `internal/plan` | parser de `show -json` (`format_version` 1.x) | rejeita state; Terraform e OpenTofu; lê `configuration` (`provider_config_key`, `region` constante do provider) |
| `internal/usage` | premissas JSON `schema_version` 1 | precedência resources > types > defaults > embutido; EKS: `extended_support_hours`, `auto_mode_instance_hours` |
| `internal/catalog` | snapshot `schema_version` 2 + `pricing refresh` (CSV em streaming) | regras de SKU em `rules.go`; `sha256` verificado no load; lê v1 e v2 |
| `internal/estimate` | fórmulas por tipo + lista de custo zero + região por recurso (`region.go`) | `resources.go`; valores em `math/big.Rat`; motivo de não precificado (`provider`/`type`/`region`/`config`/`price`) |
| `internal/report` | gate (exit 0/1/2/3), Markdown com marcador, JSON `schema_version` 1, JSON Lines | chaves da fatura por linha; `warnings` (`region_mismatch`) e `ignored` só quando se aplicam |
| `pricing/us-east-1.json`, `pricing/us-west-2.json` | snapshots embutidos (`go:embed *.json`) | 2684 e 2648 preços, as of 2026-11-20 (inclui 1261/1244 taxas do EKS Auto Mode) |
| `scripts/pricing-diff.sh` | sh + `jq` | compara só o mapa `prices` com um ref; exit 0 igual, 1 mudou (usado pelo `refresh-pricing.yml`) |
| `site/` | landing estática (Cloudflare Pages, stack `site-cdn-dns-central/cf-pages/costgate-site`) | `ping.js`: 1 evento por aba, sem cookie, filtros de robô; só a página mede |
| `scripts/adoption-stats.sh` | sh + `gh` + `jq` | métricas de adoção do mantenedor; dado no repo privado `costgate-metrics` |
| CI | GitHub Actions: `ci.yml` (lint + matriz nativa de teste linux/darwin × amd64/arm64), `release.yml` (cosign keyless), `install-smoke.yml` (brew, go install e tarball nas 4 plataformas), `refresh-pricing.yml` (semanal, todas as regiões embutidas; PR só se `prices` mudar, branch única `pricing/refresh`; `dry_run_pr` prova o PR) | actions por SHA |

## Estado atual (onde parei)

> Última atualização: 2026-11-20 (7ª sessão, sem commit: mudanças na branch `feat/workshop-findings`)

7ª sessão (achados da avaliação no workshop AWS Spark on EKS, propostas P1, P5, P6, P2 e P3 de
`aws-spark-on-eks-workshop/extra-02-avaliacao-ferramentas/kubecost-costgate-insights.md`):
região por recurso pelo plan (atributo `region` → `region` constante do provider →
ARN → ARNs dos outros recursos do mesmo provider), recurso em outra região vira não precificado
com motivo `region` e o relatório ganha o aviso `region_mismatch` com a correção; gp3 acima de
3000 IOPS e 125 MiB/s precificado (`EBS:VolumeP-IOPS.gp3`, `EBS:VolumeP-Throughput.gp3`);
snapshot us-west-2 embutido, `pricing verify` confere todos, refresh semanal abre PR só quando
`prices` muda; motivo `provider` separado de `type` e `--ignore-provider`; `aws_eks_cluster`
precificado (control plane US$ 0,10/h, suporte estendido e taxa do Auto Mode só por premissa,
SKUs `EKS-Auto:<tipo>-management-hours` da Bulk API). Totais dos goldens antigos iguais; só
`as_of`, `sha256` e a fonte AmazonEKS do snapshot de fixture mudaram, mais `unpriced_reason` no
JSON do caso `unpriced`. PRs de refresh #3 e #5 fechados pelo orquestrador (só `as_of`). Sem
release.

6ª sessão (métricas de adoção, decisões do operador de 2026-11-05): `scripts/adoption-stats.sh`
coleta traffic, downloads, stars e os jobs do nosso CI (descontados), e o repo privado
`leosavio/costgate-metrics` guarda a série com launchd semanal (`com.tars.costgate-adoption`).
`site/` é a landing com beacon para o ping (serviços `costgate-readme`, `costgate-github`,
`costgate-site`), testada em Chrome headless: robô e `webdriver` não contam, 5 s de página visível
contam. Publicação no Cloudflare Pages aguarda apply no `site-cdn-dns-central` (branch
`feat/costgate-site`); o link `?ref=readme` entra no README depois do site no ar.

5ª sessão: `docs/how-it-works.md` (cinco diagramas Mermaid: visão geral, refresh do snapshot,
`diff`, gate e CI, mais o NAT do baseline de ponta a ponta) e `docs/comparison.md` (costgate ×
Infracost CLI 2.x e Cloud, conferida em 2026-11-04 com fonte por linha). READMEs EN e PT-BR ganharam
"How it works" e "costgate and Infracost". Diagramas renderizados com `@mermaid-js/mermaid-cli` 11
antes do commit. Sem release (só docs).


4ª sessão (issue leosavio-org#26, camada 1): **v0.2.0** publicada. Cada linha de custo do JSON
ganhou `sku`, `usage_type` bruto, `rate_code`, `billing_description` e `billing_rule`
(`fixed_hourly`/`fixed_monthly`/`usage`); `--json-lines` grava um documento por linha; o Markdown
ganhou o bloco recolhido "Billing keys". Snapshot `schema_version` 2 (rate code, usage type bruto,
`service_name`, `location`, `tiers`); o load lê v1 e v2 e confere a versão antes dos campos. Totais
dos goldens iguais aos da v0.1.1; o snapshot real regerado tem a mesma primeira faixa em todos os
1415 preços. O dry run do refresh agora registra o run de `ci` do PR de teste (criado em
`action_required`, doc do `GITHUB_TOKEN`) e o apaga num job com `actions: write`. Release: tap
atualizado pelo `release.yml`, `install-smoke` disparou por `workflow_run` e passou nas 12 células.
Camadas 2 e 3 (`record`, `ledger`, conciliação) aguardam decisão do operador.


3ª sessão (issue leosavio-org#23): `ci.yml` testa nativo em `ubuntu-24.04`, `ubuntu-24.04-arm`,
`macos-15` e `macos-15-intel` (goldens byte a byte em todas); `install-smoke.yml` passou nas 12
células (4 plataformas × brew/go install/tarball) para v0.1.1; `refresh-pricing.yml -f dry_run_pr=true`
abriu e fechou o PR #1 (autor `github-actions[bot]`, label `pricing-refresh-test`). Homebrew ≥ 7.0.11
exige `brew trust --formula leosavio/tap/costgate` (documentado no README). Sem release nova (só CI e docs).

v0.1.0 publicada na 1ª sessão (MVP com 14 famílias de recurso do baseline T0–T3, goldens
idênticos em arm64/amd64). 2ª sessão: README oficial em inglês com `README.pt-BR.md`
(regra do portfólio invalidada para docs públicas, evolution-log do `leosavio-org`), versão
correta via `go install` (`debug.ReadBuildInfo`), tap `leosavio/homebrew-tap` com fórmula gerada
por `scripts/homebrew-formula.sh` e publicada pelo `release.yml` via deploy key
(`HOMEBREW_TAP_DEPLOY_KEY`, escrita só no tap). Actions pode abrir PR (refresh de preço).
Operador confirmou D4/D5 da ADR-001. Integração com o foundation **só documentada**.

## Próximos passos

- [ ] Foundation: errata na ADR-011 §2 (Infracost → costgate) e job de custo a partir de `docs/integration/github-actions.md`
- [ ] Release com as mudanças da 7ª sessão (região, gp3, us-west-2, `--ignore-provider`, EKS); revalidar no plan do workshop com `--region us-west-2`
- [ ] Resolver `region = var.<nome>` do provider pelo `variables` do plan (hoje só constante, ARN ou recursos irmãos)
- [ ] P4 do workshop: disco raiz do NodeClass/EC2NodeClass (`kubectl_manifest`) por premissa
- [ ] Resolver `subnet_ids` desconhecido pela seção `configuration` do plan (hoje usa `az_count`)
- [ ] ASG + launch template (`desired_capacity` × tipo)
- [ ] Mapear T3-03 Network Firewall, T3-06 Route 53 Profiles, T3-08 IPAM Advanced e VPC Encryption Controls quando um módulo os usar
- [ ] Reconferir `docs/comparison.md` nas fontes do Infracost a cada release do costgate (afirmação sobre terceiro, datada 2026-11-04)
- [ ] homebrew-core quando o repo tiver ≥ 30 dias e 75 stars, 30 forks ou 30 watchers (fórmula compilando do fonte)

## Comandos essenciais

```sh
go vet ./... && ~/go/bin/staticcheck ./... && go test -race -count=1 ./...
go test ./internal/cli -update                       # regrava goldens; revisar diff
GOARCH=amd64 go test ./...                           # determinismo cross-arch (Rosetta)
go run ./cmd/costgate pricing refresh                # baixa a Bulk API pública (~19 s), grava pricing/us-east-1.json
go run ./cmd/costgate pricing refresh --region us-west-2 --out pricing/us-west-2.json
go run ./cmd/costgate pricing verify                 # confere todos os snapshots embutidos
sh scripts/pricing-diff.sh HEAD                      # o que o refresh mudou em `prices` (exit 1 = mudou)
go run ./cmd/costgate diff --plan testdata/cases/baseline/plan.json --usage testdata/cases/baseline/usage.json --max-monthly-delta 50 --json-lines /tmp/lines.jsonl
git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z   # release.yml: binários, checksums, assinatura, fórmula no tap
sh scripts/homebrew-formula.sh X.Y.Z checksums.txt    # fórmula manual (fallback sem deploy key)
```

## Fontes de verdade locais

| Artefato | Caminho |
|---|---|
| ADRs | docs/adr/ |
| Integração de CI (consumidor) | docs/integration/github-actions.md |
| Funcionamento interno (diagramas) | docs/how-it-works.md |
| Comparação com o Infracost (datada, com fontes) | docs/comparison.md |
| Mapa recurso → SKU | internal/catalog/rules.go · internal/estimate/resources.go · README.md §Recursos cobertos |
| Tech debt | docs/tech-debt.md (a criar no primeiro débito) |

## Guardrails

- Nenhuma chamada a conta AWS: só a Bulk API pública. Credencial AWS nunca entra no repo nem no CI.
- Só stdlib no `go.mod`. Dinheiro sempre em `internal/money` (`big.Rat`), nunca `float64`.
- Saída do `diff` não depende de relógio, ordem de mapa nem host (`as_of` vem das offer files).
- Tipo desconhecido é não precificado (exit 2). Não adicionar tipo à lista de custo zero sem fonte.
- Snapshot só via `pricing refresh`; edição à mão quebra o `sha256` e o load recusa.
- Nada que dependa de recurso exclusivo do Terraform: o formato lido é o comum com o OpenTofu.
- Actions por SHA com tag em comentário, `permissions:` mínimas, `persist-credentials: false`.
- O binário nunca envia dado (promessa de `docs/how-it-works.md`). Medição de adoção só na landing
  (`site/ping.js`, sem cookie) e na API do GitHub pelo mantenedor; nada de telemetria no CLI.
