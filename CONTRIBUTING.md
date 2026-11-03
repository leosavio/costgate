# Contribuindo

PR e issue são bem-vindos. Regras curtas:

| Tema | Regra |
|---|---|
| Dependências | só stdlib. Dependência nova precisa de justificativa no PR |
| Determinismo | nada de relógio, mapa iterado sem ordenar ou `float64` em valor monetário; use `internal/money` |
| Tipo novo | fórmula em `internal/estimate/resources.go`, regra de SKU em `internal/catalog/rules.go`, linha no `README.md` e caso em `testdata/cases/` |
| Tipo sem custo | entrada em `zeroCostTypes` com fonte da AWS no PR |
| Golden files | `go test ./internal/cli -update` e revise o diff; o CI roda em amd64 contra goldens gerados em arm64 |
| Verificação | `gofmt -l .`, `go vet ./...`, `staticcheck ./...`, `go test -race ./...` verdes |
| Workflow | action só por SHA de commit completo com o tag em comentário; `permissions:` mínimas; `persist-credentials: false` |
| Commits | mensagens em inglês, no imperativo (`feat:`, `fix:`, `docs:`, `chore:`) |

Decisões de arquitetura ficam em `docs/adr/` (ADR aceita não é reescrita; mudança por ADR nova).
