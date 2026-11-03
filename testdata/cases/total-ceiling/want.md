<!-- costgate -->
### costgate: ❌ -US$ 51.70/month · over ceiling

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 73.52 | 21.82 | **-51.70** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-total` | 5.00 | ❌ 21.82 |
| Unpriced resources created or changed | 0 | ✅ 0 |

**Exit 1:** monthly total after the change above --max-monthly-total.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_ebs_volume.data` | update | 1.60 | 4.00 | +2.40 | EBS volume (gp3): 50 GB-month × $0.08 = 4.00 |
| `aws_eip.nat` | delete | 3.65 | – | -3.65 | public IPv4 address: 730 h × $0.005 = 3.65 |
| `aws_instance.app` | replace | 8.23 | 6.19 | -2.05 | compute Spot, Linux t4g.small: 730 h × $0.0168 × 0.4 = 4.91 (on-demand × (1 − spot_discount 0.6))<br>root volume (gp3): 16 GB-month × $0.08 = 1.28 |
| `aws_instance.batch` | update | 25.68 | 10.13 | -15.55 | compute on-demand, Linux t4g.small: 240 h × $0.0168 = 4.03<br>root volume (gp3): 20 GB-month × $0.08 = 1.60<br>ebs_block_device[0] (st1): 100 GB-month × $0.045 = 4.50 |
| `aws_nat_gateway.main` | delete | 32.85 | – | -32.85 | NAT gateway hours: 730 h × $0.045 = 32.85<br>NAT data processed: usage not declared (`gb_per_month`) |

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

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-03T00:09:47Z (sha256 `383a4cb3421f`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
