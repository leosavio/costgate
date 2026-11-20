<!-- costgate -->
### costgate: ⚠️ +US$ 2685.41/month · total ≥ US$ 2758.41/month · unpriced resources

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 73.00 | 2758.41 | **+2685.41** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 3000.00 | ✅ +2685.41 |
| Unpriced resources created or changed | 0 | ❌ 1 |

**Exit 2:** 1 resource created or changed has no known price.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_eks_cluster.main` | update | 73.00 | 2555.00 | +2482.00 | cluster control plane, standard support: 730 h × $0.1 = 73.00 (extended support not included; declare extended_support_hours for a version in extended support)<br>provisioned control plane, tier-2xl: 730 h × $3.4 = 2482.00 (billed on top of the control plane hour) |
| `aws_eks_node_group.batch` | create | – | 8.91 | +8.91 | compute Spot, Linux t4g.micro: 1460 h × $0.0084 × 0.4 = 4.91 (2 nodes from scaling_config.desired_size, which an autoscaler changes after apply; on-demand × (1 − spot_discount 0.6))<br>node disks (gp2): 40 GB-month × $0.1 = 4.00 (20 GB × 2 nodes; disk_size not set: EKS default 20 GB; gp2, the volume type of the launch template EKS creates) |
| `aws_eks_node_group.general` | create | – | 193.70 | +193.70 | compute on-demand, Linux m7g.large: 2190 h × $0.0816 = 178.70 (first of 2 instance_types; 3 nodes from scaling_config.desired_size, which an autoscaler changes after apply)<br>node disks (gp2): 150 GB-month × $0.1 = 15.00 (50 GB × 3 nodes; gp2, the volume type of the launch template EKS creates) |
| `aws_eks_node_group.windows` | create | – | ? | ? | unpriced: ami_type "WINDOWS_CORE_2022_x86_64" is not modeled (only Amazon Linux and Bottlerocket) |
| `aws_secretsmanager_secret.db` | create | – | 0.80 | +0.80 | secret: 1 secret-month × $0.4 = 0.40 (API requests not priced)<br>replica secrets: 1 secret-month × $0.4 = 0.40 (at the price of the primary region; each replica is billed in its region) |

<details><summary>Billing keys of 8 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_eks_cluster.main` | cluster control plane, standard support | `ZYWMR684YSMFHWEU` | `USE1-AmazonEKS-Hours:perCluster` | fixed_hourly | Amazon EKS cluster usage in US East (N. Virginia) |
| `aws_eks_cluster.main` | provisioned control plane, tier-2xl | `ZFR5JCZGFAZC95YM` | `USE1-AmazonEKS-Hours:2XL-ProvisionedTier` | fixed_hourly | $3.40 per Hour for 2XL Provisioned Tier in US East (N. Virginia) |
| `aws_eks_node_group.batch` | compute Spot, Linux t4g.micro | `6U8TKDT6A9WNJNP5` | `BoxUsage:t4g.micro` | usage | $0.0084 per On Demand Linux t4g.micro Instance Hour |
| `aws_eks_node_group.batch` | node disks (gp2) | `HY3BZPP2B6K8MSJF` | `EBS:VolumeUsage.gp2` | fixed_monthly | $0.10 per GB-month of General Purpose SSD (gp2) provisioned storage - US East (Northern Virginia) |
| `aws_eks_node_group.general` | compute on-demand, Linux m7g.large | `Y4JWUCG93UEP6MPT` | `BoxUsage:m7g.large` | fixed_hourly | $0.0816 per On Demand Linux m7g.large Instance Hour |
| `aws_eks_node_group.general` | node disks (gp2) | `HY3BZPP2B6K8MSJF` | `EBS:VolumeUsage.gp2` | fixed_monthly | $0.10 per GB-month of General Purpose SSD (gp2) provisioned storage - US East (Northern Virginia) |
| `aws_secretsmanager_secret.db` | secret | `BJ3PQ9BYGU6P632F` | `USE1-AWSSecretsManager-Secrets` | fixed_monthly | $0.40 per Secret |
| `aws_secretsmanager_secret.db` | replica secrets | `BJ3PQ9BYGU6P632F` | `USE1-AWSSecretsManager-Secrets` | fixed_monthly | $0.40 per Secret |

</details>

#### Unpriced resources

| Resource | Action | Reason | Blocks the gate |
|---|---|---|---|
| `aws_eks_node_group.windows` | create | ami_type "WINDOWS_CORE_2022_x86_64" is not modeled (only Amazon Linux and Bottlerocket) | yes |

<details><summary>1 changed resource with no charge</summary>

- `aws_secretsmanager_secret_version.db` (create)

</details>

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-11T19:56:06Z (sha256 `c2ef4ad32776`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
