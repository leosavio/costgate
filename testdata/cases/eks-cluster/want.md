<!-- costgate -->
### costgate: ✅ +US$ 103.38/month · total US$ 614.38/month

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 511.00 | 614.38 | **+103.38** |

| Check | Limit | Result |
|---|---:|---|
| Unpriced resources created or changed | 0 | ✅ 0 |

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_eks_cluster.batch` | update | 73.00 | 103.38 | +30.38 | cluster control plane, standard support: 730 h × $0.1 = 73.00 (extended support not included; declare extended_support_hours for a version in extended support)<br>EKS Auto Mode management fee, c6a.xlarge: 1460 h × $0.01836 = 26.81 (EC2 cost of the nodes not included)<br>EKS Auto Mode management fee, m7g.large: 365 h × $0.00979 = 3.57 (EC2 cost of the nodes not included) |
| `module.eks.aws_eks_cluster.this[0]` | create | – | 73.00 | +73.00 | cluster control plane, standard support: 730 h × $0.1 = 73.00 (extended support not included; declare extended_support_hours for a version in extended support)<br>EKS Auto Mode management fee: usage not declared (`auto_mode_instance_hours`) |

<details><summary>Billing keys of 5 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_eks_cluster.batch` | cluster control plane, standard support | `ZYWMR684YSMFHWEU` | `USE1-AmazonEKS-Hours:perCluster` | fixed_hourly | Amazon EKS cluster usage in US East (N. Virginia) |
| `aws_eks_cluster.batch` | EKS Auto Mode management fee, c6a.xlarge | `G2MWUDHPEB34JGP8` | `EKS-Auto:c6a.xlarge-management-hours` | usage | $0.01836 per hour for EKS Auto Mode management of c6a.xlarge in US East (N. Virginia) |
| `aws_eks_cluster.batch` | EKS Auto Mode management fee, m7g.large | `3VD4G6MNTAHUNRBM` | `EKS-Auto:m7g.large-management-hours` | usage | $0.00979 per hour for EKS Auto Mode management of m7g.large in US East (N. Virginia) |
| `module.eks.aws_eks_cluster.this[0]` | cluster control plane, standard support | `ZYWMR684YSMFHWEU` | `USE1-AmazonEKS-Hours:perCluster` | fixed_hourly | Amazon EKS cluster usage in US East (N. Virginia) |
| `module.eks.aws_eks_cluster.this[0]` | EKS Auto Mode management fee | `–` | `–` | usage | – |

</details>

#### Usage premises not declared (counted as 0)

- `module.eks.aws_eks_cluster.this[0]`: EKS Auto Mode management fee, set `auto_mode_instance_hours` in the usage file

<details><summary>1 unchanged priced resource</summary>

| Resource | Monthly |
|---|---:|
| `aws_eks_cluster.legacy` | 438.00 |

</details>

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-11T15:35:57Z (sha256 `15582beb60a1`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
