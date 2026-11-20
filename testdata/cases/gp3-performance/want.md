<!-- costgate -->
### costgate: ❌ +US$ 234.57/month · total US$ 242.57/month · over ceiling

| Monthly cost (USD) | Before | After | Delta |
|---|---:|---:|---:|
| Total | 8.00 | 242.57 | **+234.57** |

| Check | Limit | Result |
|---|---:|---|
| `--max-monthly-delta` | 200.00 | ❌ +234.57 |
| Unpriced resources created or changed | 0 | ✅ 0 |

**Exit 1:** monthly delta above --max-monthly-delta.

#### Changed resources

| Resource | Action | Before | After | Delta | Breakdown |
|---|---|---:|---:|---:|---|
| `aws_ebs_volume.logs` | update | 8.00 | 13.00 | +5.00 | EBS volume (gp3): 100 GB-month × $0.08 = 8.00<br>EBS volume throughput above 125 MiB/s (gp3): 0.1220703125 GiBps-month × $40.96 = 5.00 (250 MiB/s provisioned, 125 included; 125 MiB/s ÷ 1024) |
| `aws_ebs_volume.shuffle` | create | – | 130.00 | +130.00 | EBS volume (gp3): 1000 GB-month × $0.08 = 80.00<br>EBS volume IOPS above 3000 (gp3): 3000 IOPS-month × $0.005 = 15.00 (6000 IOPS provisioned, 3000 included)<br>EBS volume throughput above 125 MiB/s (gp3): 0.8544921875 GiBps-month × $40.96 = 35.00 (1000 MiB/s provisioned, 125 included; 875 MiB/s ÷ 1024) |
| `aws_instance.worker` | create | – | 99.57 | +99.57 | compute on-demand, Linux m7g.large: 730 h × $0.0816 = 59.57<br>root volume (gp3): 50 GB-month × $0.08 = 4.00<br>root volume IOPS above 3000 (gp3): 1000 IOPS-month × $0.005 = 5.00 (4000 IOPS provisioned, 3000 included)<br>ebs_block_device[0] (gp3): 200 GB-month × $0.08 = 16.00<br>ebs_block_device[0] throughput above 125 MiB/s (gp3): 0.3662109375 GiBps-month × $40.96 = 15.00 (500 MiB/s provisioned, 125 included; 375 MiB/s ÷ 1024) |

<details><summary>Billing keys of 10 cost lines (SKU and invoice description)</summary>

| Resource | Line | SKU | Usage type | Billing rule | Invoice description |
|---|---|---|---|---|---|
| `aws_ebs_volume.logs` | EBS volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_ebs_volume.logs` | EBS volume throughput above 125 MiB/s (gp3) | `SQUFRQX4K92S4SBB` | `EBS:VolumeP-Throughput.gp3` | fixed_monthly | $0.04 per provisioned MiBps-month of gp3 - US East (N. Virginia) |
| `aws_ebs_volume.shuffle` | EBS volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_ebs_volume.shuffle` | EBS volume IOPS above 3000 (gp3) | `7Q58NR58VQEASA4W` | `EBS:VolumeP-IOPS.gp3` | fixed_monthly | $0.005 per provisioned IOPS-month of gp3 - US East (N. Virginia) |
| `aws_ebs_volume.shuffle` | EBS volume throughput above 125 MiB/s (gp3) | `SQUFRQX4K92S4SBB` | `EBS:VolumeP-Throughput.gp3` | fixed_monthly | $0.04 per provisioned MiBps-month of gp3 - US East (N. Virginia) |
| `aws_instance.worker` | compute on-demand, Linux m7g.large | `Y4JWUCG93UEP6MPT` | `BoxUsage:m7g.large` | fixed_hourly | $0.0816 per On Demand Linux m7g.large Instance Hour |
| `aws_instance.worker` | root volume (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_instance.worker` | root volume IOPS above 3000 (gp3) | `7Q58NR58VQEASA4W` | `EBS:VolumeP-IOPS.gp3` | fixed_monthly | $0.005 per provisioned IOPS-month of gp3 - US East (N. Virginia) |
| `aws_instance.worker` | ebs_block_device[0] (gp3) | `JG3KUJMBRGHV3N8G` | `EBS:VolumeUsage.gp3` | fixed_monthly | $0.08 per GB-month of General Purpose (gp3) provisioned storage - US East (N. Virginia) |
| `aws_instance.worker` | ebs_block_device[0] throughput above 125 MiB/s (gp3) | `SQUFRQX4K92S4SBB` | `EBS:VolumeP-Throughput.gp3` | fixed_monthly | $0.04 per provisioned MiBps-month of gp3 - US East (N. Virginia) |

</details>

<sub>costgate test · AWS Price List Bulk API snapshot `us-east-1` as of 2026-11-11T19:56:06Z (sha256 `c2ef4ad32776`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>
