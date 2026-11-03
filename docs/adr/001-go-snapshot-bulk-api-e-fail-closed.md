# ADR-001: Go, snapshot da Price List Bulk API e fail-closed para recurso sem preço

## Status

Aceito (2026-11-03). Decisões D1–D3 tomadas pelo operador em 2026-11-02/03; D4–D6 são
detalhes de implementação decididos pelo agente dentro desse escopo.

## Contexto

- O produto `foundation-org-leo-plan` precisa barrar, no PR, mudança de Terraform que aumente o
  custo mensal acima de um teto (ADR-011 do foundation, custo em três camadas).
- Investigação de 2026-11-02 (fora do git): Infracost 0.10 está em EOL, o 2.x exige login no
  serviço, OpenInfraQuote foi arquivado, terracost exige MySQL e forks do Terraform. Nenhuma
  alternativa ativa é offline, sem SaaS e fail-closed.
- Os arquivos da AWS Price List Bulk API são públicos e não exigem credencial. O EC2 de us-east-1
  tem 303 MB em CSV; os demais serviços do MVP somam menos de 1 MB.
- A investigação recomendava Python stdlib dentro do foundation. O operador decidiu por repo
  próprio, MIT, em Go.

## Decisão

| # | Decisão |
|---|---|
| D1 | **Go**, só stdlib, binário único sem runtime, cross-compile darwin/linux × arm64/amd64. Valores monetários em `math/big.Rat`: `float64` pode ser fundido em FMA de forma diferente entre arm64 e amd64 e quebraria a saída byte a byte |
| D2 | Fonte de preço: **snapshot versionado** `pricing/<região>.json`, extraído da **Bulk API** pública por `costgate pricing refresh` e embutido no binário (`go:embed`). Só os SKUs mapeados entram; a offer file inteira nunca é versionada. O snapshot carrega `schema_version`, `as_of` (maior `publication_date` das fontes, não o relógio) e `sha256` do conteúdo; `Load` recusa hash divergente e `schema_version` maior. Query API (com credencial) fica como modo opcional futuro |
| D3 | **Fail-closed**: recurso criado, alterado ou substituído cujo custo não é conhecido sai com **exit 2**. O consumidor decide liberar (label `cost-approved` no PR); a ferramenta não conhece a label |
| D4 | Precedência de saída: 3 (erro de uso ou entrada) > 2 (não precificado) > 1 (acima do teto) > 0. Estimativa incompleta não prova que o teto foi respeitado |
| D5 | Recurso não precificado em `no-op` ou `delete` **não** bloqueia: o delta dele é ≤ 0 ou nulo, e bloquear forçaria `cost-approved` em todo PR de uma stack com recurso legado fora do mapa. Com `--max-monthly-total`, qualquer não precificado bloqueia, porque o total fica incompleto |
| D6 | Premissas de uso em **JSON** (`schema_version: 1`), não YAML: dispensa dependência fora da stdlib. Precedência por campo: `resources[endereço]` > `resources[endereço sem índice]` > `types[tipo]` > `defaults` > embutido (730 h, Spot 0, 2 AZs, raiz 8 GB) |

## Alternativas rejeitadas

| Alternativa | Motivo |
|---|---|
| Python stdlib dentro do foundation | decisão do operador: repo próprio e binário único reutilizável fora do foundation |
| Query API (`pricing:GetProducts`) no PR | exige credencial AWS no job de PR e rede no gate; o snapshot é auditável no diff |
| Ler a offer file JSON | 482 MB para o EC2; o CSV equivalente é lido em streaming com `encoding/csv` |
| Mostrar 0 para recurso desconhecido | é o defeito das alternativas: subestima sem avisar |
| `float64` | não determinístico entre arquiteturas (FMA) |
| YAML para premissas | primeira dependência externa do `go.mod` sem ganho funcional |

## Consequências

- Preço muda por PR revisado (`refresh-pricing.yml`, semanal), igual a versão de ferramenta.
- Cada regra de `internal/catalog/rules.go` precisa casar ao menos um SKU; se a AWS renomear um
  usage type, o refresh falha alto em vez de gravar snapshot incompleto.
- Tipo novo no plan exige mapeamento (fórmula + regra) ou entrada na lista de custo zero; até lá
  o PR sai com exit 2.
- Só us-east-1 é versionado. `--region` existe; outra região exige `pricing refresh --region` e
  o mapa de prefixos de usage type em `rules.go` (hoje us-east-1/2 e us-west-1/2).
- Compatível com OpenTofu: lê apenas o formato `format_version` 1.x de `show -json`, comum aos dois.
