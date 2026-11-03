# costgate

🇺🇸 [English](README.md) (versão oficial) · 🇧🇷 Português

> Tradução do `README.md`. Em caso de divergência, vale a versão em inglês.

Projeção do custo mensal da AWS de um `terraform plan` (ou `tofu plan`), **antes × depois** por
recurso, com gate de PR: sai com código distinto quando o delta passa do teto ou quando aparece
recurso sem preço conhecido. Binário único em Go, offline, sem credencial AWS e sem SaaS.

- Preços vêm de um **snapshot versionado** da [AWS Price List Bulk API](https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/price-changes.html) pública, embutido no binário. Mudança de preço entra por PR, com diff auditável.
- **Fail-closed**: recurso criado ou alterado que a ferramenta não sabe precificar falha o check (exit 2). Nenhum recurso desconhecido vira "US$ 0" em silêncio.
- **Determinístico**: mesma entrada, mesma saída byte a byte, em arm64 e amd64 (aritmética decimal exata com `math/big`).
- Saída em Markdown (comentário de PR), JSON e JSON Lines. Cada linha de custo traz as chaves da
  fatura da AWS (SKU, usage type, rate code, descrição da fatura), para casar com o Cost and Usage
  Report.

Decisões: [ADR-001 (Go, snapshot da Bulk API e fail-closed)](docs/adr/001-go-snapshot-bulk-api-e-fail-closed.md).

## Instalação

Release (darwin/linux, arm64/amd64), com verificação de checksum e assinatura keyless:

```sh
v=0.2.0; os=darwin; arch=arm64
base="https://github.com/leosavio/costgate/releases/download/v${v}"
curl -fsSLO "${base}/costgate_${v}_${os}_${arch}.tar.gz"
curl -fsSLO "${base}/checksums.txt"
curl -fsSLO "${base}/checksums.txt.sigstore.json"
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/leosavio/costgate/.github/workflows/release.yml@refs/tags/v${v}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
grep " costgate_${v}_${os}_${arch}.tar.gz\$" checksums.txt | shasum -a 256 -c -
tar -xzf "costgate_${v}_${os}_${arch}.tar.gz" costgate
```

No Linux, `sha256sum -c -` faz o mesmo que `shasum -a 256 -c -`.

Homebrew (macOS e Linux), pelo tap [`leosavio/homebrew-tap`](https://github.com/leosavio/homebrew-tap):

```sh
brew install leosavio/tap/costgate
```

O Homebrew 7.0.11 em diante recusa fórmula de tap não oficial até ela ser confiada. Se o
`brew install` parar com "Refusing to load formula ... from untrusted tap", rode
`brew trust --formula leosavio/tap/costgate` e instale de novo.

Com Go instalado: `go install github.com/leosavio/costgate/cmd/costgate@latest`.

### Plataformas testadas

| Plataforma | Runner do GitHub | `go test -race` + goldens (`ci.yml`) | Smoke de instalação (`install-smoke.yml`) |
|---|---|---|---|
| linux/amd64 | `ubuntu-24.04` | nativo | brew · go install · tarball |
| linux/arm64 | `ubuntu-24.04-arm` | nativo | brew · go install · tarball |
| darwin/arm64 | `macos-15` | nativo | brew · go install · tarball |
| darwin/amd64 | `macos-15-intel` | nativo (CPU Intel, não Rosetta) | brew · go install · tarball |

O [`install-smoke.yml`](.github/workflows/install-smoke.yml) instala uma release publicada como o
usuário faria e roda após cada release, toda terça e sob demanda
(`gh workflow run install-smoke.yml -f version=vX.Y.Z`). Em cada caminho confere que
`costgate version` bate com a tag, que `costgate pricing verify` passa e que o caso
`testdata/cases/baseline` sai com exit 1 e o Markdown do golden
([`scripts/smoke-check.sh`](scripts/smoke-check.sh)).

### Verificar a instalação

O script compara com o testdata da tag instalada:

```sh
git clone -q --depth 1 --branch "v${v}" https://github.com/leosavio/costgate costgate-src && cd costgate-src
curl -fsSLO https://raw.githubusercontent.com/leosavio/costgate/main/scripts/smoke-check.sh
sh smoke-check.sh "$(command -v costgate)" "${v}"
# ok version: costgate 0.2.0
# ok us-east-1: 1415 prices, ...
# ok baseline: exit 1, ### costgate: ❌ +US$ 414.16/month · over ceiling
```

## Uso

```sh
terraform plan -out=tfplan
terraform show -json tfplan > plan.json      # tofu show -json também serve

costgate diff --plan plan.json --usage costgate-usage.json --max-monthly-delta 10
costgate diff --plan - --format json < plan.json
```

| Flag | Padrão | Efeito |
|---|---|---|
| `--plan` | obrigatório | saída de `show -json` de um **plan** (`-` lê do stdin) |
| `--usage` | nenhum | arquivo de premissas de uso (abaixo) |
| `--max-monthly-delta` | sem teto | exit 1 se `depois − antes` > valor (USD/mês) |
| `--max-monthly-total` | sem teto | exit 1 se o total depois da mudança > valor; com recurso não precificado, exit 2 |
| `--region` | `us-east-1` | região dos preços; o snapshot embutido é só us-east-1 |
| `--pricing` | embutido | outro snapshot (gerado por `pricing refresh`) |
| `--format` | `markdown` | `markdown` ou `json` no stdout |
| `--markdown-out`, `--json-out` | — | grava também em arquivo |
| `--json-lines` | — | grava também um documento JSON por linha de custo ([JSON Lines](#json-lines)) |

| Exit | Significado |
|---|---|
| 0 | tudo precificado e dentro dos tetos |
| 1 | acima de `--max-monthly-delta` ou `--max-monthly-total` |
| 2 | recurso criado, alterado ou substituído sem preço conhecido (tem precedência sobre 1) |
| 3 | erro de uso ou de entrada: flag, plan inválido, state no lugar de plan, premissa ou snapshot inválido |

Recurso não precificado em `no-op` ou `delete` aparece no relatório mas não bloqueia: o delta dele
não é positivo. A liberação por label (`cost-approved`) é do consumidor; a ferramenta só devolve o
código. Job de exemplo: [docs/integration/github-actions.md](docs/integration/github-actions.md).

## Premissas de uso

O plan não traz volume (GB, horas ligadas, desconto Spot). O arquivo de premissas é JSON,
versionado junto com a stack ([exemplo](examples/usage.json)):

```json
{
  "schema_version": 1,
  "defaults": {"az_count": 2},
  "types": {"aws_nat_gateway": {"gb_per_month": 50}},
  "resources": {
    "aws_instance.worker": {"hours_per_month": 200, "spot_discount": 0.7},
    "module.net.aws_vpc_endpoint.ssm": {"gb_per_month": 10}
  }
}
```

| Campo | Padrão embutido | Uso |
|---|---|---|
| `hours_per_month` | 730 | todo item por hora (0–744) |
| `gb_per_month` | não declarado (0, sinalizado) | dados de NAT, endpoint e TGW; armazenamento S3 |
| `spot_discount` | 0 | Spot = on-demand × (1 − desconto), 0 ≤ d < 1. O padrão 0 é o teto conservador |
| `events_per_month` | não declarado (0, sinalizado) | eventos pagos do CloudTrail (a 1ª cópia de management events é grátis) |
| `az_count` | 2 | subnets de interface endpoint quando o plan não sabe a contagem |
| `root_volume_gb` | 8 | volume raiz de instância quando o plan não sabe o tamanho |

Precedência, campo a campo: `resources["endereço[índice]"]` > `resources["endereço"]` (sem
`[índice]`) > `types[tipo]` > `defaults` > embutido. Premissa de volume não declarada conta 0 e é
listada no relatório.

## Recursos cobertos (snapshot us-east-1 de 2026-11-03)

| Tipo Terraform | Fórmula | SKU (usage type, operação) | Preço |
|---|---|---|---|
| `aws_nat_gateway` | 730 h + GB | `NatGateway-Hours`, `NatGateway-Bytes` (`NatGateway`) | 0,045/h + 0,045/GB |
| `aws_eip`, `associate_public_ip_address` em `aws_instance` | 730 h por IP | `USE1-PublicIPv4:InUseAddress` | 0,005/h |
| `aws_vpc_endpoint` `Interface` (`Gateway` = 0) | 730 h × nº de subnets + GB | `USE1-VpcEndpoint-Hours`, `-Bytes` (`VpcEndpoint`) | 0,01/h por AZ + 0,01/GB |
| `aws_ec2_transit_gateway_vpc_attachment` | 730 h + GB | `USE1-TransitGateway-Hours`, `-Bytes` (`TransitGatewayVPC`) | 0,05/h + 0,02/GB |
| `aws_ec2_transit_gateway_peering_attachment` | 730 h | `USE1-TransitGateway-Hours` (`TransitGatewayPeering`) | 0,05/h |
| `aws_vpn_connection` | 730 h; + attachment se `transit_gateway_id` | `VPN-Usage-Hours:ipsec.1`; `USE1-TransitGateway-Hours`/`-Bytes` (`TransitGatewayVPN`) | 0,05/h (+ 0,05/h + 0,02/GB) |
| `aws_route53_zone` | por zona/mês | `HostedZone` (offer global) | 0,50 (faixa das 25 primeiras) |
| `aws_route53_resolver_endpoint` | 730 h × nº de `ip_address` (mín. 2) | `USE1-ResolverNetworkInterface` | 0,125/h por ENI |
| `aws_kms_key`, `aws_kms_replica_key`, `aws_kms_external_key`, `aws_kms_replica_external_key` | por chave/mês | `us-east-1-KMS-Keys` | 1,00 |
| `aws_instance`, `aws_spot_instance_request` | 730 h × preço do tipo (Spot com desconto) + raiz + `ebs_block_device` | `BoxUsage:<tipo>` (`RunInstances`, Linux, shared) — 1 392 tipos, x86 e Graviton | ex.: t3.micro 0,0104/h, t4g.micro 0,0084/h |
| `aws_ebs_volume`, volumes de instância | GB-mês | `EBS:VolumeUsage[.gp2/.gp3/.st1/.sc1]` | gp3 0,08, gp2 0,10, st1 0,045, sc1 0,015, standard 0,05 |
| `aws_s3_bucket` | GB-mês (premissa) | `TimedStorage-ByteHrs` | 0,023 (primeiros 50 TB) |
| `aws_cloudtrail` | eventos pagos (premissa) | `USE1-PaidEventsRecorded` | 0,00002 por evento |
| `aws_acmpca_certificate_authority` | por CA/mês conforme `usage_mode` | `USE1-PaidPrivateCA`, `USE1-ShortLivedCertificatePrivateCA` | 400 / 50 |
| `aws_acm_certificate` (público, não exportável), `aws_accessanalyzer_analyzer` (external access) | — | sem cobrança | 0 |

Tipos sem custo próprio (VPC, subnet, rotas, security groups, IAM, Organizations, Identity
Center, RAM, `aws_ec2_transit_gateway`, `aws_route53_record`, configurações de bucket S3,
`random_*`, `time_*`…) estão na lista de custo zero em
[`internal/estimate/resources.go`](internal/estimate/resources.go). Qualquer outro tipo é **não
precificado** (exit 2 se criado ou alterado). Caso também não precificado, por decisão: NAT
regional, endpoint `GatewayLoadBalancer`/`Resource`, EBS io1/io2 e gp3 acima de 3000 IOPS ou
125 MiB/s, tenancy dedicada, VPN de túnel `large`, CloudTrail com data events ou Insights, recurso
com `region` diferente da precificada, `instance_type` desconhecido no plan.

## Exemplo de comentário de PR

Gerado de [`testdata/cases/changes`](testdata/cases/changes) com o snapshot embutido
(`costgate diff --plan plan.json --usage usage.json --max-monthly-delta 50`):

<!-- costgate -->
### costgate: ✅ -US$ 51.70/month

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 73.52 | 21.82 | **-51.70** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 50.00 | ✅ -51.70 |
| Unpriced resources created or changed | 0 | ✅ 0 |

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_ebs_volume.data` | update | 1.60 | 4.00 | +2.40 | EBS volume (gp3): 50 GB-month × $0.08 = 4.00 |
| `aws_eip.nat` | delete | 3.65 | – | -3.65 | public IPv4 address: 730 h × $0.005 = 3.65 |
| `aws_instance.app` | replace | 8.23 | 6.19 | -2.05 | compute Spot, Linux t4g.small: 730 h × $0.0168 × 0.4 = 4.91 (on-demand × (1 − spot_discount 0.6))<br>root volume (gp3): 16 GB-month × $0.08 = 1.28 |
| `aws_instance.batch` | update | 25.68 | 10.13 | -15.55 | compute on-demand, Linux t4g.small: 240 h × $0.0168 = 4.03<br>root volume (gp3): 20 GB-month × $0.08 = 1.60<br>ebs_block_device[0] (st1): 100 GB-month × $0.045 = 4.50 |
| `aws_nat_gateway.main` | delete | 32.85 | – | -32.85 | NAT gateway hours: 730 h × $0.045 = 32.85<br>NAT data processed: usage not declared (`gb_per_month`) |

<details><summary>Billing keys of 9 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_ebs_volume.data` | EBS volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_eip.nat` | public IPv4 address | `4GQUNXTFWVSGPUZK` | `USE1-PublicIPv4:InUseAddress` | fixed_hourly | $0.005 per In-use public IPv4 address per hour |
| `aws_instance.app` | compute Spot, Linux t4g.small | `9GWX7R3VJUG9P8Y8` | `BoxUsage:t4g.small` | usage | $0.0168 per On Demand Linux t4g.small Instance Hour |
| `aws_instance.app` | root volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_instance.batch` | compute on-demand, Linux t4g.small | `9GWX7R3VJUG9P8Y8` | `BoxUsage:t4g.small` | fixed_hourly | $0.0168 per On Demand Linux t4g.small Instance Hour |
| `aws_instance.batch` | root volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_instance.batch` | ebs_block_device[0] (st1) | `YQT3C842QHBG6XCU` | `EBS:VolumeUsage.st1` | fixed_monthly | $0.045 per GB-month of Throughput Optimized HDD (st1) provisioned storage - US East (Northern Virginia) |
| `aws_nat_gateway.main` | NAT gateway hours | `M2YSHUBETB3JX4M4` | `NatGateway-Hours` | fixed_hourly | $0.045 per NAT Gateway Hour |
| `aws_nat_gateway.main` | NAT data processed | `59S5R83GFPUAGVR5` | `NatGateway-Bytes` | usage | $0.045 per GB Data Processed by NAT Gateways |

</details>

#### Usage premises not declared (counted as 0)

- `aws_nat_gateway.main`: NAT data processed, set `gb_per_month` in the usage file

<details><summary>1 changed resource with no charge</summary>

- `aws_route_table.private` (update)

</details>

<details><summary>2 unchanged priced resources</summary>

| Resource | Monthly |
|---|---:|
| `aws_kms_key.state` | 1.00 |
| `module.dns.aws_route53_zone.this["example.com"]` | 0.50 |

</details>

<sub>costgate 0.2.0 · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-03T00:09:47Z (sha256 `b27de59b0883`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>

## Relatório JSON

`--format json` (stdout) e `--json-out` (arquivo) gravam o relatório completo, `schema_version` 1.
Dentro de um `schema_version` só entram campos novos; remover campo ou mudar significado sobe a
versão. Cada elemento de `resources[].before.lines` e `resources[].after.lines` é uma linha de custo:

| Campo | Desde | Significado |
|---|---|---|
| `name`, `unit`, `quantity`, `unit_price`, `factor`, `monthly` | 0.1.0 | a conta mostrada na coluna Breakdown |
| `price_key` | 0.1.0 | `offer code\|usage type sem prefixo de região\|operation`, chave do snapshot |
| `premise`, `declared`, `note` | 0.1.0 | premissa de uso por trás da quantidade e se o arquivo de premissas a declarou |
| `sku` | 0.2.0 | SKU da Price List, igual ao `product_sku` do CUR 2.0 |
| `usage_type` | 0.2.0 | usage type bruto, com prefixo de região, como na fatura (`USE1-PublicIPv4:InUseAddress`, `us-east-1-KMS-Keys`; EC2 e S3 não têm prefixo em us-east-1) |
| `rate_code` | 0.2.0 | rate code da faixa usada na estimativa (a primeira), igual ao `pricing_rate_code` do CUR 2.0 |
| `billing_description` | 0.2.0 | descrição do preço, o texto que a AWS imprime na linha da fatura |
| `billing_rule` | 0.2.0 | `fixed_hourly`, `fixed_monthly` ou `usage` (abaixo) |

| `billing_rule` | A AWS cobra | Linhas |
|---|---|---|
| `fixed_hourly` | por hora em que o recurso existe | NAT gateway, IPv4 pública, endpoint interface, attachments de TGW, VPN, ENIs do Resolver, instância on-demand |
| `fixed_monthly` | por mês em que o recurso existe | hosted zone, chave KMS, Private CA, GB-mês provisionado de EBS |
| `usage` | por tráfego, dado armazenado, evento ou preço de mercado; a quantidade ou o preço é premissa | dado processado, armazenamento S3, eventos do CloudTrail, instância Spot |

As linhas `fixed_*` são as que devem casar com a fatura por SKU e quantidade. A projeção usa
730 h/mês; a fatura usa as horas do mês do calendário, então o valor mensal difere mesmo quando a
linha casa. Com snapshot gravado pelo costgate 0.1.x (`schema_version` 1), `usage_type` e
`rate_code` saem como string vazia.

### JSON Lines

`--json-lines <arquivo>` grava um documento JSON compacto por linha de custo e por lado (`before`,
depois `after`), na ordem do relatório: o formato que o JSON SerDe do Athena lê
([Athena: JSON](https://docs.aws.amazon.com/athena/latest/ug/parsing-json-data.html)) e que
`jq -c` produz. Cada documento é autocontido: repete `schema_version`, a versão do costgate, data e
hash do snapshot, e endereço, tipo e ação do recurso. Recurso não precificado ou de custo zero não
tem linha de custo; aparece só no relatório JSON. Uma linha de `testdata/cases/changes` com o
snapshot embutido:

```json
{"schema_version":1,"kind":"costgate.line","tool":"costgate","version":"0.2.0","region":"us-east-1","currency":"USD","pricing_as_of":"2026-11-03T00:09:47Z","pricing_sha256":"b27de59b08836c188cfa5bb1c758b2246392b90280def70f26d1c49f27daa0be","address":"module.dns.aws_route53_zone.this[\"example.com\"]","type":"aws_route53_zone","action":"no-op","side":"after","name":"hosted zone","price_key":"AmazonRoute53|HostedZone|","sku":"KVPGEJE88UW8S779","usage_type":"HostedZone","rate_code":"KVPGEJE88UW8S779.JRTCKXETXF.G2A93NZ7DT","billing_description":"$0.50 per Hosted Zone for the first 25 Hosted Zones","billing_rule":"fixed_monthly","unit_price":0.5,"quantity":1,"unit":"zone-month","monthly":0.5000,"declared":true,"note":"first-25-zones tier"}
```

## Atualizar preços

```sh
costgate pricing refresh --region us-east-1 --out pricing/us-east-1.json   # HTTP público, sem credencial
costgate pricing verify --pricing pricing/us-east-1.json
```

O refresh lê `offers/v1.0/aws/index.json`, baixa o CSV de cada serviço mapeado (o do EC2 tem
~300 MB e é lido em streaming) e grava só os SKUs das regras de
[`internal/catalog/rules.go`](internal/catalog/rules.go), com `schema_version`, versão e data de
cada offer file e `sha256`. Regra que não casa nenhum SKU, ou que casa dois SKUs com preços
diferentes, falha o refresh. O workflow `refresh-pricing.yml` faz isso toda segunda e abre PR
quando algum preço muda.

O snapshot é `schema_version` 2 desde o costgate 0.2.0: cada preço guarda também `rate_code`,
`usage_type` bruto, `service_name` e `location` da offer file, e as faixas depois da primeira
(`tiers`, para referência; a estimativa usa a primeira faixa). O snapshot vai embutido no binário,
então cada release precifica com a própria cópia; `--pricing` lê um arquivo no lugar.

| Binário | Snapshot embutido | Arquivo `--pricing` v1 | Arquivo `--pricing` v2 |
|---|---|---|---|
| 0.1.x | v1 | lê | recusa: `json: unknown field "rate_code"` (exit 3) |
| 0.2.x | v2 | lê; `usage_type` e `rate_code` vazios | lê |

O `Load` recusa campo desconhecido e `schema_version` fora da faixa que o build conhece, então
binário antigo nunca lê pela metade um snapshot mais novo. Desde a 0.2.0 ele confere a versão antes
dos campos, então um arquivo v3 futuro falha com `schema_version 3 not supported` em vez do nome de
um campo.

Para provar o caminho do PR sem tocar `main` nem o snapshot real, rode em modo de teste:

```sh
gh workflow run refresh-pricing.yml -f dry_run_pr=true
```

O run faz o refresh e os testes normais, adiciona um campo sintético `test_marker` ao snapshot
numa branch descartável `pricing/refresh-test-<run id>`, abre PR com label `pricing-refresh-test`,
confere que o autor é `github-actions[bot]` e, no mesmo run, fecha o PR e apaga a branch (também
quando uma verificação falha). Mudança real de preço achada num dry run não vira PR; o próximo run
semanal a pega.

PR aberto com `GITHUB_TOKEN` recebe o run de `ci` em estado de aprovação pendente
([doc do GitHub](https://docs.github.com/en/actions/concepts/security/github_token)): aprove pelo
PR antes de fazer merge de um refresh real. No dry run esse run não resolve mais o ref depois que a
branch é apagada e termina como falha sem jobs; o dry run registra o run no resumo do job e um job
separado (o único com `actions: write`) o apaga.

## Limitações

- **Uso variável é premissa**, não medição: GB processado, horas ligadas, eventos e
  armazenamento valem o que o arquivo de premissas diz. Sem premissa, conta 0 e o relatório avisa.
- Preço de **lista on-demand**. Savings Plans, Reserved Instances, créditos, Free Tier, descontos
  negociados e impostos ficam fora. Spot é `on-demand × (1 − spot_discount)`, não o preço Spot
  do momento (que exige credencial e varia por AZ).
- Só a **primeira faixa** de preço escalonado entra na conta (hosted zone: 25 primeiras; S3: 50 TB;
  endpoint: 1 PB). O snapshot guarda as faixas seguintes, e a fatura pode cobrar uma delas porque a
  AWS conta a faixa na organização inteira; compare `rate_code` com `pricing_rate_code`.
- Não cobre transferência de dados entre AZs/regiões/internet, requisições (KMS, S3, Route 53
  queries), certificados emitidos pela Private CA, nem IOPS provisionado.
- Só Linux em tenancy compartilhada; AMI Windows/RHEL/SUSE é precificada como Linux (o plan não diz
  o sistema operacional da AMI).
- Quando o plan não conhece a lista de subnets de um endpoint (`subnet_ids` vindo de outro
  recurso), usa `az_count`. ASG e launch template ainda não são resolvidos: `aws_autoscaling_group`
  é não precificado.
- Valores por recurso são arredondados para exibição; os totais usam a soma exata, então
  `depois − antes` de uma linha pode diferir 0,01 da coluna Delta.
- Só us-east-1 é versionado.

## Desenvolvimento

```sh
go vet ./... && staticcheck ./... && go test -race ./...
go test ./internal/cli -update      # regrava golden files de testdata/ (revise o diff)
```

Guia curto: [CONTRIBUTING.md](CONTRIBUTING.md) (em inglês). Licença: [MIT](LICENSE).
