<!-- costgate -->
### costgate: ⚠️ +US$ 1.00/month · total ≥ US$ 1.00/month · unpriced resources

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 0.00 | 1.00 | **+1.00** |

| Check | Limit | Result |
|---|---:|---|
| Unpriced resources created or changed | 0 | ❌ 2 |

**Exit 2:** 2 resources created or changed have no known price.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_instance.i` | create | – | ? | ? | unpriced: no price for AmazonEC2\|BoxUsage:</script><img src=x onerror=alert(1)>\|RunInstances in the snapshot |
| `aws_kms_key.app["</script><img src=x onerror=alert(1)>"]` | create | – | 1.00 | +1.00 | customer managed key: 1 key-month × $1 = 1.00 (requests not priced) |
| `aws_s3_bucket.b["<img src=x onerror=alert(1)>"]` | create | – | 0.00 | 0.00 | S3 Standard storage: usage not declared (`gb_per_month`) |
| `module.m["a.b</script><img src=x onerror=alert(1)>"].aws_kms_key.k` | create | – | ? | ? | unpriced: provider "registry.terraform.io/evil/</script><img src=x onerror=alert(1)>" is not the AWS provider; --ignore-provider script><img src=x onerror=alert(1)> leaves it out of the gate |

<details><summary>Billing keys of 2 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_kms_key.app["</script><img src=x onerror=alert(1)>"]` | customer managed key | `U553K98XGDXCYHWS` | `us-east-1-KMS-Keys` | fixed_monthly | $1 per customer managed KMS key version in US East (N. Virginia) |
| `aws_s3_bucket.b["<img src=x onerror=alert(1)>"]` | S3 Standard storage | `WP9ANXZGBYYSGJEA` | `TimedStorage-ByteHrs` | usage | $0.023 per GB - first 50 TB / month of storage used |

</details>

#### Unpriced resources

| Resource | Action | Reason | Blocks the gate |
|---|---|---|---|
| `aws_instance.i` | create | no price for AmazonEC2\|BoxUsage:</script><img src=x onerror=alert(1)>\|RunInstances in the snapshot | yes |
| `module.m["a.b</script><img src=x onerror=alert(1)>"].aws_kms_key.k` | create | provider "registry.terraform.io/evil/</script><img src=x onerror=alert(1)>" is not the AWS provider; --ignore-provider script><img src=x onerror=alert(1)> leaves it out of the gate | yes |

#### Usage premises not declared (counted as 0)

- `aws_s3_bucket.b["<img src=x onerror=alert(1)>"]`: S3 Standard storage, set `gb_per_month` in the usage file

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-11T19:56:06Z (sha256 `c2ef4ad32776`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
