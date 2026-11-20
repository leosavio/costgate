<!-- costgate -->
### costgate: ✅ +US$ 57.92/month

> ⚠️ **Warning (`region_assumed`):** 1 resource states no region in the plan (no region attribute, no region in the provider configuration, no ARN yet); priced in us-east-1, the default, because the plan names several regions (eu-west-1, us-east-1).<br>**Fix:** rerun with --region <region> if they deploy elsewhere.

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 3.65 | 61.57 | **+57.92** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 100.00 | ✅ +57.92 |
| Unpriced resources created or changed | 0 | ✅ 0 |

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_eip.nat` · eu-west-1 | create | – | 4.56 | +4.56 | public IPv4 address: 730 h × $0.00625 = 4.56 |
| `aws_instance.app` · eu-west-1 | create | – | 10.29 | +10.29 | compute on-demand, Linux t3.micro: 730 h × $0.013 = 9.49<br>root volume (gp3): 8 GB-month × $0.1 = 0.80 |
| `aws_kms_key.cloudfront` · us-east-1 | create | – | 1.00 | +1.00 | customer managed key: 1 key-month × $1 = 1.00 (requests not priced) |
| `aws_nat_gateway.this` · eu-west-1 | create | – | 41.06 | +41.06 | NAT gateway hours: 730 h × $0.05625 = 41.06<br>NAT data processed: usage not declared (`gb_per_month`) |
| `module.legacy.aws_kms_key.this` · us-east-1 | create | – | 1.00 | +1.00 | customer managed key: 1 key-month × $1 = 1.00 (requests not priced) |

<details><summary>Billing keys of 7 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_eip.nat` | public IPv4 address | `4GQUNXTFWVSGPUZK` | `EU-PublicIPv4:InUseAddress` | fixed_hourly | $0.005 per In-use public IPv4 address per hour |
| `aws_instance.app` | compute on-demand, Linux t3.micro | `CRAJUW7BTXFMT2UJ` | `EU-BoxUsage:t3.micro` | fixed_hourly | $0.0104 per On Demand Linux t3.micro Instance Hour |
| `aws_instance.app` | root volume (gp3) | `JG3KUJMBRGHV3N8G` | `EU-EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - EU (Ireland) |
| `aws_kms_key.cloudfront` | customer managed key | `U553K98XGDXCYHWS` | `us-east-1-KMS-Keys` | fixed_monthly | $1 per customer managed KMS key version in US East (N. Virginia) |
| `aws_nat_gateway.this` | NAT gateway hours | `M2YSHUBETB3JX4M4` | `EU-NatGateway-Hours` | fixed_hourly | $0.045 per NAT Gateway Hour |
| `aws_nat_gateway.this` | NAT data processed | `59S5R83GFPUAGVR5` | `EU-NatGateway-Bytes` | usage | $0.045 per GB Data Processed by NAT Gateways |
| `module.legacy.aws_kms_key.this` | customer managed key | `U553K98XGDXCYHWS` | `us-east-1-KMS-Keys` | fixed_monthly | $1 per customer managed KMS key version in US East (N. Virginia) |

</details>

#### Usage premises not declared (counted as 0)

- `aws_nat_gateway.this`: NAT data processed, set `gb_per_month` in the usage file

<details><summary>1 unchanged priced resource</summary>

| Resource | Monthly |
|---|---:|
| `aws_eip.edge` · us-east-1 | 3.65 |

</details>

<sub>costgate test · AWS Price List Bulk API snapshots `eu-west-1` as of 2026-11-11T15:35:57Z (sha256 `a69153b222e0`), `us-east-1` as of 2026-11-11T15:35:57Z (sha256 `15582beb60a1`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
