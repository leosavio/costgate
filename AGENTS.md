---
type: guide
summary: "Contrato de contexto do repo costgate para agentes: CLI Go (MIT) que projeta o custo mensal AWS de um terraform/tofu plan com snapshot da Price List Bulk API e gate fail-closed de PR."
updated: 2026-11-03
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
| `internal/plan` | parser de `show -json` (`format_version` 1.x) | rejeita state; Terraform e OpenTofu |
| `internal/usage` | premissas JSON `schema_version` 1 | precedência resources > types > defaults > embutido |
| `internal/catalog` | snapshot + `pricing refresh` (CSV em streaming) | regras de SKU em `rules.go`; `sha256` verificado no load |
| `internal/estimate` | fórmulas por tipo + lista de custo zero | `resources.go`; valores em `math/big.Rat` |
| `internal/report` | gate (exit 0/1/2/3), Markdown com marcador, JSON `schema_version` 1 | |
| `pricing/us-east-1.json` | snapshot embutido (`go:embed`) | 1415 preços, as of 2026-11-03 |
| CI | GitHub Actions: `ci.yml`, `release.yml` (cosign keyless), `refresh-pricing.yml` (semanal) | actions por SHA |

## Estado atual (onde parei)

> Última atualização: 2026-11-03

MVP criado na sessão de 2026-11-03 (kiro-cli): 14 famílias de recurso do baseline T0–T3 do
foundation, Spot com desconto, Graviton, `create/update/delete/replace/no-op/forget`, golden
tests em `testdata/cases/` (baseline vindo de um plan real do Terraform 1.14.5 + provider AWS
6.58.0, gerado offline com credencial falsa) e fixture da Bulk API em `testdata/offers/`.
Goldens idênticos em arm64 e amd64. Release v0.1.0. Integração com o foundation **só
documentada** (`docs/integration/github-actions.md`), não aplicada.

## Próximos passos

- [ ] Operador: habilitar "Allow GitHub Actions to create and approve pull requests" para o `refresh-pricing.yml`
- [ ] Foundation: errata na ADR-011 §2 (Infracost → costgate) e job de custo a partir de `docs/integration/github-actions.md`
- [ ] Resolver `subnet_ids` desconhecido pela seção `configuration` do plan (hoje usa `az_count`)
- [ ] ASG + launch template (`desired_capacity` × tipo)
- [ ] Mapear T3-03 Network Firewall, T3-06 Route 53 Profiles, T3-08 IPAM Advanced e VPC Encryption Controls quando um módulo os usar

## Comandos essenciais

```sh
go vet ./... && ~/go/bin/staticcheck ./... && go test -race -count=1 ./...
go test ./internal/cli -update                       # regrava goldens; revisar diff
GOARCH=amd64 go test ./...                           # determinismo cross-arch (Rosetta)
go run ./cmd/costgate pricing refresh                # baixa a Bulk API pública (~12 s), grava pricing/us-east-1.json
go run ./cmd/costgate pricing verify
go run ./cmd/costgate diff --plan testdata/cases/baseline/plan.json --usage testdata/cases/baseline/usage.json --max-monthly-delta 50
git tag vX.Y.Z && git push origin vX.Y.Z             # release.yml: binários + checksums.txt + .sigstore.json
```

## Fontes de verdade locais

| Artefato | Caminho |
|---|---|
| ADRs | docs/adr/ |
| Integração de CI (consumidor) | docs/integration/github-actions.md |
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
