<!-- costgate -->
### costgate: ⚠️ +US$ 3.65/month · total ≥ US$ 3.65/month · unpriced resources

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 0.00 | 3.65 | **+3.65** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 10.00 | ✅ +3.65 |
| Unpriced resources created or changed | 0 | ❌ 2 |

**Exit 2:** 2 resources created or changed have no known price.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_eip.ingress` | create | – | 3.65 | +3.65 | public IPv4 address: 730 h × $0.005 = 3.65 |
| `module.eks_data_addons.helm_release.kubecost[0]` | update | ? | ? | ? | unpriced: provider "registry.terraform.io/hashicorp/helm" is not the AWS provider; --ignore-provider helm leaves it out of the gate |
| `module.eks_data_addons.kubectl_manifest.nodeclass["spark"]` | create | – | ? | ? | unpriced: provider "registry.terraform.io/alekc/kubectl" is not the AWS provider; --ignore-provider kubectl leaves it out of the gate |

<details><summary>Billing keys of 1 cost line (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_eip.ingress` | public IPv4 address | `4GQUNXTFWVSGPUZK` | `USE1-PublicIPv4:InUseAddress` | fixed_hourly | $0.005 per In-use public IPv4 address per hour |

</details>

#### Unpriced resources

| Resource | Action | Reason | Blocks the gate |
|---|---|---|---|
| `aws_lb.legacy` | no-op | resource type not in the pricing map | no (unchanged or deleted) |
| `module.eks_data_addons.helm_release.kubecost[0]` | update | provider "registry.terraform.io/hashicorp/helm" is not the AWS provider; --ignore-provider helm leaves it out of the gate | yes |
| `module.eks_data_addons.kubectl_manifest.nodeclass["spark"]` | create | provider "registry.terraform.io/alekc/kubectl" is not the AWS provider; --ignore-provider kubectl leaves it out of the gate | yes |
| `module.eks_data_addons.kubernetes_namespace.spark` | no-op | provider "registry.terraform.io/hashicorp/kubernetes" is not the AWS provider; --ignore-provider kubernetes leaves it out of the gate | no (unchanged or deleted) |

<details><summary>1 changed resource with no charge</summary>

- `random_password.grafana` (create)

</details>

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-11T19:56:06Z (sha256 `c2ef4ad32776`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
