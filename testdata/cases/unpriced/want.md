<!-- costgate -->
### costgate: ⚠️ +US$ 1.00/month · unpriced resources

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 0.00 | 1.00 | **+1.00** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 10.00 | ✅ +1.00 |
| Unpriced resources created or changed | 0 | ❌ 4 |

**Exit 2:** 4 resources created or changed have no known price.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_db_instance.old` | delete | ? | – | ? | unpriced: resource type not in the pricing map |
| `aws_instance.from_var` | create | – | ? | ? | unpriced: instance_type unknown at plan time |
| `aws_kms_key.app` | create | – | 1.00 | +1.00 | customer managed key: 1 key-month × $1 = 1.00 (requests not priced) |
| `aws_nat_gateway.west` | create | – | ? | ? | unpriced: resource region us-west-2 differs from priced region us-east-1 |
| `aws_networkfirewall_firewall.inspection` | create | – | ? | ? | unpriced: resource type not in the pricing map |
| `aws_s3_bucket.logs` | create | – | 0.00 | 0.00 | S3 Standard storage: usage not declared (`gb_per_month`) |
| `aws_vpc_endpoint.gwlb` | create | – | ? | ? | unpriced: vpc_endpoint_type "GatewayLoadBalancer" is not modeled |

#### Unpriced resources

| Resource | Action | Reason | Blocks the gate |
|---|---|---|---|
| `aws_db_instance.old` | delete | resource type not in the pricing map | no (unchanged or deleted) |
| `aws_instance.from_var` | create | instance_type unknown at plan time | yes |
| `aws_lb.legacy` | no-op | resource type not in the pricing map | no (unchanged or deleted) |
| `aws_nat_gateway.west` | create | resource region us-west-2 differs from priced region us-east-1 | yes |
| `aws_networkfirewall_firewall.inspection` | create | resource type not in the pricing map | yes |
| `aws_vpc_endpoint.gwlb` | create | vpc_endpoint_type "GatewayLoadBalancer" is not modeled | yes |

#### Usage premises not declared (counted as 0)

- `aws_s3_bucket.logs`: S3 Standard storage, set `gb_per_month` in the usage file

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-03T00:09:47Z (sha256 `383a4cb3421f`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
