<!-- costgate -->
### costgate: ❌ +US$ 414.16/month · over ceiling

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 0.00 | 414.16 | **+414.16** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 50.00 | ❌ +414.16 |
| Unpriced resources created or changed | 0 | ✅ 0 |

**Exit 1:** monthly delta above --max-monthly-delta.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_acmpca_certificate_authority.internal` | create | – | 50.00 | +50.00 | Private CA (short-lived mode): 1 CA-month × $50 = 50.00 (certificates not priced) |
| `aws_cloudtrail.org` | create | – | 0.00 | 0.00 | paid events recorded: 0 events × $0.00002 = 0.00 (first copy of management events is free; declare only paid events) |
| `aws_ebs_volume.data` | create | – | 1.60 | +1.60 | EBS volume (gp3): 20 GB-month × $0.08 = 1.60 |
| `aws_ec2_transit_gateway_vpc_attachment.main` | create | – | 38.50 | +38.50 | TGW VPC attachment hours: 730 h × $0.05 = 36.50<br>TGW data processed: 100 GB × $0.02 = 2.00 |
| `aws_eip.nat` | create | – | 3.65 | +3.65 | public IPv4 address: 730 h × $0.005 = 3.65 |
| `aws_instance.web` | create | – | 11.88 | +11.88 | compute on-demand, Linux t3.micro: 730 h × $0.0104 = 7.59<br>public IPv4 address: 730 h × $0.005 = 3.65<br>root volume (gp3): 8 GB-month × $0.08 = 0.64 |
| `aws_instance.worker` | create | – | 1.30 | +1.30 | compute Spot, Linux t4g.micro: 200 h × $0.0084 × 0.3 = 0.50 (on-demand × (1 − spot_discount 0.7))<br>root volume (gp2): 8 GB-month × $0.1 = 0.80 (root size unknown at plan time; root_volume_gb premise 8 GB (built-in); root type unknown; assumed gp2) |
| `aws_kms_key.state` | create | – | 1.00 | +1.00 | customer managed key: 1 key-month × $1 = 1.00 (requests not priced) |
| `aws_nat_gateway.main` | create | – | 35.10 | +35.10 | NAT gateway hours: 730 h × $0.045 = 32.85<br>NAT data processed: 50 GB × $0.045 = 2.25 |
| `aws_route53_resolver_endpoint.inbound` | create | – | 182.50 | +182.50 | Resolver ENI hours × 2: 1460 h × $0.125 = 182.50 |
| `aws_route53_zone.private` | create | – | 0.50 | +0.50 | hosted zone: 1 zone-month × $0.5 = 0.50 (first-25-zones tier) |
| `aws_route53_zone.public` | create | – | 0.50 | +0.50 | hosted zone: 1 zone-month × $0.5 = 0.50 (first-25-zones tier) |
| `aws_s3_bucket.trail` | create | – | 0.02 | +0.02 | S3 Standard storage: 1 GB-month × $0.023 = 0.02 (first 50 TB tier; requests not priced) |
| `aws_vpc_endpoint.ssm` | create | – | 14.60 | +14.60 | endpoint hours × 2 AZ: 1460 h × $0.01 = 14.60 (subnet count unknown at plan time; az_count premise 2 (defaults))<br>endpoint data processed: usage not declared (`gb_per_month`) |
| `aws_vpn_connection.onprem` | create | – | 73.00 | +73.00 | VPN connection hours: 730 h × $0.05 = 36.50<br>TGW VPN attachment hours: 730 h × $0.05 = 36.50<br>TGW VPN data processed: usage not declared (`gb_per_month`) |

#### Usage premises not declared (counted as 0)

- `aws_vpc_endpoint.ssm`: endpoint data processed, set `gb_per_month` in the usage file
- `aws_vpn_connection.onprem`: TGW VPN data processed, set `gb_per_month` in the usage file

<details><summary>7 changed resources with no charge</summary>

- `aws_customer_gateway.onprem` (create)
- `aws_ec2_transit_gateway.hub` (create)
- `aws_security_group.resolver` (create)
- `aws_subnet.private[0]` (create)
- `aws_subnet.private[1]` (create)
- `aws_vpc.main` (create)
- `aws_vpc_endpoint.s3` (create)

</details>

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-03T00:09:47Z (sha256 `383a4cb3421f`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
