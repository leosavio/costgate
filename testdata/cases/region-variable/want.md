<!-- costgate -->
### costgate: ✅ +US$ 16.10/month · total US$ 16.10/month

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 0.00 | 16.10 | **+16.10** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 50.00 | ✅ +16.10 |
| Unpriced resources created or changed | 0 | ✅ 0 |

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_eip.app` | create | – | 4.56 | +4.56 | public IPv4 address: 730 h × $0.00625 = 4.56 |
| `aws_instance.app` | create | – | 10.29 | +10.29 | compute on-demand, Linux t3.micro: 730 h × $0.013 = 9.49<br>root volume (gp3): 8 GB-month × $0.1 = 0.80 |
| `aws_kms_key.app` | create | – | 1.25 | +1.25 | customer managed key: 1 key-month × $1.25 = 1.25 (requests not priced) |

<details><summary>Billing keys of 4 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_eip.app` | public IPv4 address | `4GQUNXTFWVSGPUZK` | `EU-PublicIPv4:InUseAddress` | fixed_hourly | $0.005 per In-use public IPv4 address per hour |
| `aws_instance.app` | compute on-demand, Linux t3.micro | `CRAJUW7BTXFMT2UJ` | `EU-BoxUsage:t3.micro` | fixed_hourly | $0.0104 per On Demand Linux t3.micro Instance Hour |
| `aws_instance.app` | root volume (gp3) | `JG3KUJMBRGHV3N8G` | `EU-EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - EU (Ireland) |
| `aws_kms_key.app` | customer managed key | `U553K98XGDXCYHWS` | `eu-west-1-KMS-Keys` | fixed_monthly | $1 per customer managed KMS key version in EU (Ireland) |

</details>

<sub>costgate test · AWS Price List Bulk API snapshot `eu-west-1` as of 2026-11-11T15:35:57Z (sha256 `a69153b222e0`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
