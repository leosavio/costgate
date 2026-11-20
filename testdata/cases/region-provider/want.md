<!-- costgate -->
### costgate: ⚠️ US$ 0.00/month · total ≥ US$ 0.00/month · unpriced resources

> ⚠️ **Warning (`region_mismatch`):** provider configuration aws deploys to us-west-2, and costgate has no price snapshot of us-west-2; 4 resources in us-west-2 left unpriced (reason region).<br>**Fix:** run 'costgate pricing refresh --region us-west-2 --out us-west-2.json.gz', then rerun with --pricing us-west-2.json.gz.

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 0.00 | 0.00 | **0.00** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 50.00 | ✅ 0.00 |
| Unpriced resources created or changed | 0 | ❌ 1 |

**Exit 2:** 1 resource created or changed has no known price.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_instance.bastion` | create | – | ? | ? | unpriced: no price snapshot for resource region us-west-2 (from provider configuration aws) |

#### Unpriced resources

| Resource | Action | Reason | Blocks the gate |
|---|---|---|---|
| `aws_instance.bastion` | create | no price snapshot for resource region us-west-2 (from provider configuration aws) | yes |
| `module.eks.module.kms.aws_kms_key.this[0]` | no-op | no price snapshot for resource region us-west-2 (from provider configuration aws) | no (unchanged or deleted) |
| `module.vpc.aws_eip.nat[0]` | no-op | no price snapshot for resource region us-west-2 (from provider configuration aws) | no (unchanged or deleted) |
| `module.vpc.aws_nat_gateway.this[0]` | no-op | no price snapshot for resource region us-west-2 (from provider configuration aws) | no (unchanged or deleted) |

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-11T19:56:06Z (sha256 `c2ef4ad32776`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
