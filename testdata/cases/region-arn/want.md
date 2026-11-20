<!-- costgate -->
### costgate: ⚠️ US$ 0.00/month · unpriced resources

> ⚠️ **Warning (`region_mismatch`):** provider configuration aws deploys to us-west-2 (region taken from the region attribute or ARNs of its resources), but prices are for us-east-1; 4 resources in us-west-2 left unpriced (reason region).<br>**Fix:** rerun with --region us-west-2 (embedded snapshot).

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
| `aws_instance.bastion` | create | – | ? | ? | unpriced: resource region us-west-2 (from ARNs of other resources of provider configuration aws) differs from priced region us-east-1 |

#### Unpriced resources

| Resource | Action | Reason | Blocks the gate |
|---|---|---|---|
| `aws_instance.bastion` | create | resource region us-west-2 (from ARNs of other resources of provider configuration aws) differs from priced region us-east-1 | yes |
| `module.eks.module.kms.aws_kms_key.this[0]` | no-op | resource region us-west-2 (from ARN) differs from priced region us-east-1 | no (unchanged or deleted) |
| `module.vpc.aws_eip.nat[0]` | no-op | resource region us-west-2 (from ARN) differs from priced region us-east-1 | no (unchanged or deleted) |
| `module.vpc.aws_nat_gateway.this[0]` | no-op | resource region us-west-2 (from ARNs of other resources of provider configuration aws) differs from priced region us-east-1 | no (unchanged or deleted) |

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-11T15:35:57Z (sha256 `15582beb60a1`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
