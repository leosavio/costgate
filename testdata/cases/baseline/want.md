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

<details><summary>Billing keys of 23 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_acmpca_certificate_authority.internal` | Private CA (short-lived mode) | `WYNC7HK4PW8UKMKD` | `USE1-ShortLivedCertificatePrivateCA` | fixed_monthly | $50 monthly charge for AWS Private CA (Short-lived certificate mode) operation in US East (N. Virginia) |
| `aws_cloudtrail.org` | paid events recorded | `HSTGM6MX75VC7UUF` | `USE1-PaidEventsRecorded` | usage | 0.00002 per paid event recorded in US East (N.Virginia) region |
| `aws_ebs_volume.data` | EBS volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_ec2_transit_gateway_vpc_attachment.main` | TGW VPC attachment hours | `9N2FHGMMCZBKDCGH` | `USE1-TransitGateway-Hours` | fixed_hourly | $0.05 per Transit Gateway VPC Attachment Hour |
| `aws_ec2_transit_gateway_vpc_attachment.main` | TGW data processed | `7W9NVPG8YS74A6WC` | `USE1-TransitGateway-Bytes` | usage | $0.02 per GB Data Processed by Transit Gateway VPC Attachment |
| `aws_eip.nat` | public IPv4 address | `4GQUNXTFWVSGPUZK` | `USE1-PublicIPv4:InUseAddress` | fixed_hourly | $0.005 per In-use public IPv4 address per hour |
| `aws_instance.web` | compute on-demand, Linux t3.micro | `CRAJUW7BTXFMT2UJ` | `BoxUsage:t3.micro` | fixed_hourly | $0.0104 per On Demand Linux t3.micro Instance Hour |
| `aws_instance.web` | public IPv4 address | `4GQUNXTFWVSGPUZK` | `USE1-PublicIPv4:InUseAddress` | fixed_hourly | $0.005 per In-use public IPv4 address per hour |
| `aws_instance.web` | root volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_instance.worker` | compute Spot, Linux t4g.micro | `6U8TKDT6A9WNJNP5` | `BoxUsage:t4g.micro` | usage | $0.0084 per On Demand Linux t4g.micro Instance Hour |
| `aws_instance.worker` | root volume (gp2) | `HY3BZPP2B6K8MSJF` | `EBS:VolumeUsage.gp2` | fixed_monthly | $0.10 per GB-month of General Purpose SSD (gp2) provisioned storage - US East (Northern Virginia) |
| `aws_kms_key.state` | customer managed key | `U553K98XGDXCYHWS` | `us-east-1-KMS-Keys` | fixed_monthly | $1 per customer managed KMS key version in US East (N. Virginia) |
| `aws_nat_gateway.main` | NAT gateway hours | `M2YSHUBETB3JX4M4` | `NatGateway-Hours` | fixed_hourly | $0.045 per NAT Gateway Hour |
| `aws_nat_gateway.main` | NAT data processed | `59S5R83GFPUAGVR5` | `NatGateway-Bytes` | usage | $0.045 per GB Data Processed by NAT Gateways |
| `aws_route53_resolver_endpoint.inbound` | Resolver ENI hours × 2 | `GZT83HBHCE5EGFNH` | `USE1-ResolverNetworkInterface` | fixed_hourly | $0.125 per hour per Resolver Network Interface |
| `aws_route53_zone.private` | hosted zone | `KVPGEJE88UW8S779` | `HostedZone` | fixed_monthly | $0.50 per Hosted Zone for the first 25 Hosted Zones |
| `aws_route53_zone.public` | hosted zone | `KVPGEJE88UW8S779` | `HostedZone` | fixed_monthly | $0.50 per Hosted Zone for the first 25 Hosted Zones |
| `aws_s3_bucket.trail` | S3 Standard storage | `WP9ANXZGBYYSGJEA` | `TimedStorage-ByteHrs` | usage | $0.023 per GB - first 50 TB / month of storage used |
| `aws_vpc_endpoint.ssm` | endpoint hours × 2 AZ | `EN2N5TATXE673A3B` | `USE1-VpcEndpoint-Hours` | fixed_hourly | $0.01 per VPC Endpoint Hour |
| `aws_vpc_endpoint.ssm` | endpoint data processed | `DQYYEZBCWSW5XS2R` | `USE1-VpcEndpoint-Bytes` | usage | $0.01 per GB for upto 1 PB monthly data processed by VPC Endpoints |
| `aws_vpn_connection.onprem` | VPN connection hours | `EC5BHP9ZANKN9EQD` | `VPN-Usage-Hours:ipsec.1` | fixed_hourly | $0.05 per VPN Connection-Hour |
| `aws_vpn_connection.onprem` | TGW VPN attachment hours | `CUP7BB8T9H2HZPVT` | `USE1-TransitGateway-Hours` | fixed_hourly | $0.05 per Transit Gateway VPN Attachment Hour |
| `aws_vpn_connection.onprem` | TGW VPN data processed | `NWMDJWA6SF7Z3MXD` | `USE1-TransitGateway-Bytes` | usage | $0.02 per GB Data Processed by Transit Gateway VPN Attachment |

</details>

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

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-03T00:09:47Z (sha256 `cbfe2a28af24`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
