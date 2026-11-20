package estimate

import (
	"strings"
	"testing"

	"github.com/leosavio/costgate/internal/catalog"
	"github.com/leosavio/costgate/internal/money"
	"github.com/leosavio/costgate/internal/plan"
	"github.com/leosavio/costgate/internal/usage"
	"github.com/leosavio/costgate/pricing"
)

func embedded(t *testing.T, region string) *catalog.Snapshot {
	t.Helper()
	data, err := pricing.Snapshot(region)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := catalog.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// Every fixed price key used by an estimator must exist in every embedded
// snapshot, or the gate would report a covered resource as unpriced.
func TestEmbeddedSnapshotHasEveryKey(t *testing.T) {
	regions := pricing.Regions()
	if len(regions) < 2 || regions[0] != "us-east-1" || regions[1] != "us-west-2" {
		t.Fatalf("embedded regions %v, want us-east-1 and us-west-2", regions)
	}
	keys := []string{keyNATHours, keyNATBytes, keyIPv4, keyEndpointHours, keyEndpointBytes, keyTGWVPCHours,
		keyTGWVPCBytes, keyTGWPeerHours, keyTGWVPNHours, keyTGWVPNBytes, keyVPNHours, keyHostedZone,
		keyResolverENI, keyKMSKey, keyS3Standard, keyTrailEvents, keyPCAGeneral, keyPCAShortLived,
		keyGP3IOPS, keyGP3Throughput, keyEKSCluster, keyEKSExtended}
	for _, ut := range ebsUsageTypes {
		keys = append(keys, catalog.Key("AmazonEC2", ut, ""))
	}
	for _, it := range []string{"t3.micro", "t4g.micro", "m7g.large", "c7g.xlarge", "m5.large"} {
		keys = append(keys, instanceKey(it))
	}
	for _, it := range []string{"c6a.xlarge", "m5.large", "m7g.large"} {
		keys = append(keys, eksAutoModeKey(it))
	}
	for _, region := range regions {
		snap := embedded(t, region)
		if snap.Region != region {
			t.Errorf("pricing/%s.json says region %s", region, snap.Region)
		}
		for _, k := range keys {
			if _, ok := snap.Lookup(k); !ok {
				t.Errorf("%s: embedded snapshot lacks %s", region, k)
			}
		}
	}
}

func parsePlan(t *testing.T, doc string) *plan.Plan {
	t.Helper()
	p, err := plan.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func byAddress(t *testing.T, res *Result) map[string]Resource {
	t.Helper()
	out := map[string]Resource{}
	for _, r := range res.Resources {
		out[r.Address] = r
	}
	return out
}

// The workshop case (P1): AWS provider 5.x, no region attribute on the
// resources, region only in the provider configuration and in the ARNs.
const regionPlan = `{
  "format_version": "1.2",
  "resource_changes": [
    {"address": "module.vpc.aws_eip.nat[0]", "mode": "managed", "type": "aws_eip", "provider_name": "registry.terraform.io/hashicorp/aws",
     "change": {"actions": ["no-op"], "before": {"domain": "vpc", "arn": "arn:aws:ec2:us-west-2:000000000000:elastic-ip/eipalloc-0000000000000000a"}, "after": {"domain": "vpc", "arn": "arn:aws:ec2:us-west-2:000000000000:elastic-ip/eipalloc-0000000000000000a"}}},
    {"address": "module.vpc.aws_nat_gateway.this[0]", "mode": "managed", "type": "aws_nat_gateway", "provider_name": "registry.terraform.io/hashicorp/aws",
     "change": {"actions": ["no-op"], "before": {"connectivity_type": "public"}, "after": {"connectivity_type": "public"}}},
    {"address": "module.kms.aws_kms_replica_key.this", "mode": "managed", "type": "aws_kms_replica_key", "provider_name": "registry.terraform.io/hashicorp/aws",
     "change": {"actions": ["no-op"], "before": {"arn": "arn:aws:kms:us-west-2:000000000000:key/mrk-0", "primary_key_arn": "arn:aws:kms:us-east-1:000000000000:key/mrk-0"}, "after": {"arn": "arn:aws:kms:us-west-2:000000000000:key/mrk-0", "primary_key_arn": "arn:aws:kms:us-east-1:000000000000:key/mrk-0"}}},
    {"address": "aws_eip.east", "mode": "managed", "type": "aws_eip", "provider_name": "registry.terraform.io/hashicorp/aws",
     "change": {"actions": ["create"], "before": null, "after": {"domain": "vpc"}, "after_unknown": {"arn": true}}},
    {"address": "aws_eip.v6", "mode": "managed", "type": "aws_eip", "provider_name": "registry.terraform.io/hashicorp/aws",
     "change": {"actions": ["create"], "before": null, "after": {"domain": "vpc", "region": "us-east-1"}}}
  ],
  "configuration": {
    "provider_config": {
      "aws": {"name": "aws", "full_name": "registry.terraform.io/hashicorp/aws", "expressions": {"region": {"references": ["local.region"]}}},
      "aws.east": {"name": "aws", "full_name": "registry.terraform.io/hashicorp/aws", "alias": "east", "expressions": {"region": {"constant_value": "us-east-1"}}}
    },
    "root_module": {
      "resources": [
        {"address": "aws_eip.east", "mode": "managed", "type": "aws_eip", "name": "east", "provider_config_key": "aws.east"}
      ],
      "module_calls": {
        "vpc": {"module": {"resources": [
          {"address": "aws_eip.nat", "mode": "managed", "type": "aws_eip", "name": "nat", "provider_config_key": "aws"},
          {"address": "aws_nat_gateway.this", "mode": "managed", "type": "aws_nat_gateway", "name": "this", "provider_config_key": "aws"}
        ]}},
        "kms": {"module": {"resources": [
          {"address": "aws_kms_replica_key.this", "mode": "managed", "type": "aws_kms_replica_key", "name": "this", "provider_config_key": "aws"}
        ]}}
      }
    }
  }
}`

func TestRegionFromPlan(t *testing.T) {
	res, err := Estimate(parsePlan(t, regionPlan), embedded(t, "us-east-1"), usage.Empty(), Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := byAddress(t, res)
	cases := map[string]string{
		// Own ARN; the replica's primary_key_arn (us-east-1) does not count.
		"module.vpc.aws_eip.nat[0]":           "resource region us-west-2 (from ARN) differs from priced region us-east-1",
		"module.kms.aws_kms_replica_key.this": "resource region us-west-2 (from ARN) differs from priced region us-east-1",
		// No ARN: the other resources of provider configuration aws are in us-west-2.
		"module.vpc.aws_nat_gateway.this[0]": "resource region us-west-2 (from ARNs of other resources of provider configuration aws) differs from priced region us-east-1",
		// Constant region of the aliased provider configuration.
		"aws_eip.east": "",
		// The region attribute (provider 6.x), no configuration entry.
		"aws_eip.v6": "",
	}
	for addr, want := range cases {
		r, ok := got[addr]
		if !ok {
			t.Fatalf("%s missing", addr)
		}
		if r.Unpriced() != want {
			t.Errorf("%s: unpriced %q, want %q", addr, r.Unpriced(), want)
		}
		if want != "" && (r.UnpricedReason() != ReasonRegion || r.Blocking) {
			t.Errorf("%s: reason %q blocking %v", addr, r.UnpricedReason(), r.Blocking)
		}
	}
	if money.Fixed(got["aws_eip.east"].After.Monthly, 2) != "3.65" {
		t.Errorf("aws_eip.east: %s", money.Fixed(got["aws_eip.east"].After.Monthly, 2))
	}
	m := res.RegionMismatches
	if len(m) != 1 || m[0].Region != "us-west-2" || !m[0].Inferred || m[0].Resources != 3 || strings.Join(m[0].Providers, ",") != "aws" {
		t.Errorf("mismatches %+v, want aws in us-west-2, inferred, 3 resources", m)
	}

	// A resource of the same configuration naming another region makes
	// the learned region ambiguous: the NAT gateway is priced as before.
	doc := strings.Replace(regionPlan, `"after": {"domain": "vpc", "region": "us-east-1"}}}`,
		`"after": {"domain": "vpc", "region": "us-east-1"}}},
    {"address": "aws_eip.other", "mode": "managed", "type": "aws_eip", "provider_name": "registry.terraform.io/hashicorp/aws",
     "change": {"actions": ["no-op"], "before": {"arn": "arn:aws:ec2:us-east-1:000000000000:elastic-ip/eipalloc-b"}, "after": {"arn": "arn:aws:ec2:us-east-1:000000000000:elastic-ip/eipalloc-b"}}}`, 1)
	doc = strings.Replace(doc, `"provider_config_key": "aws.east"}`, `"provider_config_key": "aws.east"},
        {"address": "aws_eip.other", "mode": "managed", "type": "aws_eip", "name": "other", "provider_config_key": "aws"}`, 1)
	res, err = Estimate(parsePlan(t, doc), embedded(t, "us-east-1"), usage.Empty(), Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if nat := byAddress(t, res)["module.vpc.aws_nat_gateway.this[0]"]; nat.Unpriced() != "" {
		t.Errorf("ambiguous configuration: nat %q", nat.Unpriced())
	}
	if len(res.RegionMismatches) != 0 {
		t.Errorf("ambiguous configuration: mismatches %+v", res.RegionMismatches)
	}
}

func TestRegionMismatchFromProviderConstant(t *testing.T) {
	doc := strings.Replace(regionPlan, `{"references": ["local.region"]}`, `{"constant_value": "us-west-2"}`, 1)
	doc = strings.Replace(doc, `"provider_config_key": "aws.east"}`, `"provider_config_key": "aws.east"},
        {"address": "aws_eip.v6", "mode": "managed", "type": "aws_eip", "name": "v6", "provider_config_key": "aws"}`, 1)
	res, err := Estimate(parsePlan(t, doc), embedded(t, "us-east-1"), usage.Empty(), Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := byAddress(t, res)
	nat := got["module.vpc.aws_nat_gateway.this[0]"]
	if nat.Unpriced() != "resource region us-west-2 (from provider configuration aws) differs from priced region us-east-1" {
		t.Errorf("nat: %q", nat.Unpriced())
	}
	if got["aws_eip.v6"].Unpriced() != "" {
		t.Errorf("region attribute must win over the provider configuration: %q", got["aws_eip.v6"].Unpriced())
	}
	want := []RegionMismatch{{Region: "us-west-2", Providers: []string{"aws"}, Resources: 3}}
	if len(res.RegionMismatches) != 1 || res.RegionMismatches[0].Region != want[0].Region ||
		strings.Join(res.RegionMismatches[0].Providers, ",") != "aws" || res.RegionMismatches[0].Inferred ||
		res.RegionMismatches[0].Resources != 3 {
		t.Errorf("mismatches %+v, want %+v", res.RegionMismatches, want)
	}
	// Priced in us-west-2, the same resources are priced with USW2 keys.
	res, err = Estimate(parsePlan(t, doc), embedded(t, "us-west-2"), usage.Empty(), Options{Region: "us-west-2"})
	if err != nil {
		t.Fatal(err)
	}
	eip := byAddress(t, res)["module.vpc.aws_eip.nat[0]"]
	if eip.Unpriced() != "" || len(eip.After.Lines) != 1 || eip.After.Lines[0].UsageType != "USW2-PublicIPv4:InUseAddress" {
		t.Errorf("us-west-2 eip: %q %+v", eip.Unpriced(), eip.After.Lines)
	}
	if !strings.HasPrefix(byAddress(t, res)["aws_eip.east"].Unpriced(), "resource region us-east-1 (from provider configuration aws.east)") {
		t.Errorf("aws_eip.east in us-west-2 run: %q", byAddress(t, res)["aws_eip.east"].Unpriced())
	}
}

func TestARNRegion(t *testing.T) {
	cases := map[string]string{
		"arn:aws:ec2:us-west-2:000000000000:elastic-ip/eipalloc-1": "us-west-2",
		"arn:aws-us-gov:kms:us-gov-west-1:000000000000:key/1":      "us-gov-west-1",
		"arn:aws:s3:::bucket":                                         "",
		"arn:aws:iam::000000000000:role/x":                            "",
		"arn:aws:route53:::hostedzone/Z1":                             "",
		"arn:aws:ec2:us-west-2":                                       "",
		"eipalloc-1":                                                  "",
		"arn:aws:logs:eu-central-1:000000000000:log-group:/a/b:*":     "eu-central-1",
		"arn:aws:ec2:not a region:000000000000:elastic-ip/eipalloc-1": "",
	}
	for in, want := range cases {
		if got := arnRegion(in); got != want {
			t.Errorf("%s -> %q, want %q", in, got, want)
		}
	}
	nested := map[string]any{"a": []any{"arn:aws:sns:us-west-2:000000000000:t"}, "b": "arn:aws:sqs:us-east-1:000000000000:q"}
	if r := ownARNRegion(nested); r != "" {
		t.Errorf("conflicting nested ARNs gave %q", r)
	}
	nested["b"] = "arn:aws:sqs:us-west-2:000000000000:q"
	if r := ownARNRegion(nested); r != "us-west-2" {
		t.Errorf("agreeing nested ARNs gave %q", r)
	}
}

func TestProviderReasonAndIgnore(t *testing.T) {
	doc := `{"format_version": "1.2", "resource_changes": [
	  {"address": "helm_release.kubecost", "mode": "managed", "type": "helm_release", "provider_name": "registry.terraform.io/hashicorp/helm",
	   "change": {"actions": ["update"], "before": {"name": "kubecost"}, "after": {"name": "kubecost"}}},
	  {"address": "kubectl_manifest.nodeclass", "mode": "managed", "type": "kubectl_manifest", "provider_name": "registry.terraform.io/alekc/kubectl",
	   "change": {"actions": ["create"], "before": null, "after": {}}},
	  {"address": "aws_lb.app", "mode": "managed", "type": "aws_lb", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["no-op"], "before": {}, "after": {}}},
	  {"address": "random_id.x", "mode": "managed", "type": "random_id", "provider_name": "registry.terraform.io/hashicorp/random",
	   "change": {"actions": ["create"], "before": null, "after": {}}}
	]}`
	snap := embedded(t, "us-east-1")
	res, err := Estimate(parsePlan(t, doc), snap, usage.Empty(), Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := byAddress(t, res)
	if r := got["helm_release.kubecost"]; r.UnpricedReason() != ReasonProvider || !r.Blocking ||
		r.Unpriced() != `provider "registry.terraform.io/hashicorp/helm" is not the AWS provider; --ignore-provider helm leaves it out of the gate` {
		t.Errorf("helm: %q %q blocking %v", r.UnpricedReason(), r.Unpriced(), r.Blocking)
	}
	if r := got["aws_lb.app"]; r.UnpricedReason() != ReasonType || r.Unpriced() != "resource type not in the pricing map" || r.Blocking {
		t.Errorf("aws_lb: %q %q blocking %v", r.UnpricedReason(), r.Unpriced(), r.Blocking)
	}
	if r := got["random_id.x"]; !r.ZeroCost || r.Unpriced() != "" {
		t.Errorf("random_id stays zero-cost: %+v", r)
	}
	if len(res.Blocking()) != 2 {
		t.Errorf("default: %d blocking, want helm and kubectl", len(res.Blocking()))
	}

	res, err = Estimate(parsePlan(t, doc), snap, usage.Empty(), Options{Region: "us-east-1", IgnoreProviders: []string{"hashicorp/helm", "kubectl"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Blocking()) != 0 {
		t.Errorf("ignored providers still block: %+v", res.Blocking())
	}
	if len(res.Ignored) != 2 || res.Ignored[0].Address != "helm_release.kubecost" || res.Ignored[1].Address != "kubectl_manifest.nodeclass" ||
		res.Ignored[1].Provider != "registry.terraform.io/alekc/kubectl" || res.Ignored[0].Action != plan.Update {
		t.Errorf("ignored: %+v", res.Ignored)
	}
	if _, ok := byAddress(t, res)["helm_release.kubecost"]; ok {
		t.Error("ignored resource still in Resources")
	}
	if !res.Incomplete() {
		t.Error("aws_lb (type not in the map) still makes the total incomplete")
	}
}

func TestGP3PerformanceAboveBaseline(t *testing.T) {
	doc := `{"format_version": "1.2", "resource_changes": [
	  {"address": "aws_ebs_volume.fast", "mode": "managed", "type": "aws_ebs_volume", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"size": 100, "type": "gp3", "iops": 6000, "throughput": 1000}}},
	  {"address": "aws_ebs_volume.base", "mode": "managed", "type": "aws_ebs_volume", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"size": 100, "type": "gp3", "iops": 3000, "throughput": 125}}},
	  {"address": "aws_ebs_volume.io2", "mode": "managed", "type": "aws_ebs_volume", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"size": 100, "type": "io2", "iops": 6000}}}
	]}`
	res, err := Estimate(parsePlan(t, doc), embedded(t, "us-east-1"), usage.Empty(), Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := byAddress(t, res)
	fast := got["aws_ebs_volume.fast"].After
	if fast.Unpriced != "" || len(fast.Lines) != 3 {
		t.Fatalf("fast: %q %+v", fast.Unpriced, fast.Lines)
	}
	want := []struct{ qty, monthly, usageType, rule string }{
		{"100", "8.00", "EBS:VolumeUsage.gp3", BillingFixedMonthly},
		{"3000", "15.00", "EBS:VolumeP-IOPS.gp3", BillingFixedMonthly},
		{"0.8544921875", "35.00", "EBS:VolumeP-Throughput.gp3", BillingFixedMonthly},
	}
	for i, w := range want {
		l := fast.Lines[i]
		if money.Trim(l.Quantity) != w.qty || money.Fixed(l.Monthly, 2) != w.monthly || l.UsageType != w.usageType || l.BillingRule != w.rule {
			t.Errorf("line %d: %s %s %s %s, want %+v", i, money.Trim(l.Quantity), money.Fixed(l.Monthly, 2), l.UsageType, l.BillingRule, w)
		}
	}
	if money.Fixed(fast.Monthly, 2) != "58.00" {
		t.Errorf("fast total %s", money.Fixed(fast.Monthly, 2))
	}
	if base := got["aws_ebs_volume.base"].After; len(base.Lines) != 1 || money.Fixed(base.Monthly, 2) != "8.00" {
		t.Errorf("baseline gp3: %+v", base.Lines)
	}
	if io2 := got["aws_ebs_volume.io2"]; io2.UnpricedReason() != ReasonConfig {
		t.Errorf("io2 stays unpriced: %q %q", io2.UnpricedReason(), io2.Unpriced())
	}
}

func TestEKSCluster(t *testing.T) {
	snap := embedded(t, "us-east-1")
	doc := `{"format_version": "1.2", "resource_changes": [
	  {"address": "aws_eks_cluster.std", "mode": "managed", "type": "aws_eks_cluster", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"name": "std", "compute_config": []}}},
	  {"address": "aws_eks_cluster.auto", "mode": "managed", "type": "aws_eks_cluster", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"name": "auto", "compute_config": [{"enabled": true, "node_pools": ["general-purpose"]}]}}},
	  {"address": "aws_eks_cluster.declared", "mode": "managed", "type": "aws_eks_cluster", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"name": "declared", "compute_config": [{"enabled": true}]}}},
	  {"address": "aws_eks_cluster.outpost", "mode": "managed", "type": "aws_eks_cluster", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"name": "o", "outpost_config": [{"control_plane_instance_type": "m5.large"}]}}},
	  {"address": "aws_eks_cluster.xl", "mode": "managed", "type": "aws_eks_cluster", "provider_name": "registry.terraform.io/hashicorp/aws",
	   "change": {"actions": ["create"], "before": null, "after": {"name": "xl", "control_plane_scaling_config": [{"tier": "tier-xl"}]}}}
	]}`
	use, err := usage.Load([]byte(`{"schema_version": 1, "resources": {"aws_eks_cluster.declared": {
	  "extended_support_hours": 730, "auto_mode_instance_hours": {"m5.large": 730, "c6a.xlarge": 1460}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Estimate(parsePlan(t, doc), snap, use, Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	got := byAddress(t, res)
	if s := got["aws_eks_cluster.std"].After; len(s.Lines) != 1 || money.Fixed(s.Monthly, 2) != "73.00" || s.Lines[0].UsageType != "USE1-AmazonEKS-Hours:perCluster" {
		t.Errorf("standard: %s %+v", money.Fixed(s.Monthly, 2), s.Lines)
	}
	auto := got["aws_eks_cluster.auto"].After
	if len(auto.Lines) != 2 || money.Fixed(auto.Monthly, 2) != "73.00" || auto.Lines[1].Declared || auto.Lines[1].PriceKey != "" ||
		auto.Lines[1].Premise != "auto_mode_instance_hours" {
		t.Errorf("auto mode without premise: %s %+v", money.Fixed(auto.Monthly, 2), auto.Lines)
	}
	und := res.Undeclared()
	if len(und) != 1 || und[0].Address != "aws_eks_cluster.auto" || und[0].Premise != "auto_mode_instance_hours" {
		t.Errorf("undeclared: %+v", und)
	}
	dec := got["aws_eks_cluster.declared"].After
	c6a, _ := snap.Lookup(eksAutoModeKey("c6a.xlarge"))
	m5, _ := snap.Lookup(eksAutoModeKey("m5.large"))
	want := money.Add(money.Add(money.Int(73), money.Int(365)), money.Add(money.Mul(c6a, money.Int(1460)), money.Mul(m5, money.Int(730))))
	if len(dec.Lines) != 4 || dec.Monthly.Cmp(want) != 0 {
		t.Errorf("declared: %s, want %s; %+v", money.Fixed(dec.Monthly, 4), money.Fixed(want, 4), dec.Lines)
	}
	if len(dec.Lines) == 4 && (dec.Lines[1].Name != "extended support" || dec.Lines[2].Name != "EKS Auto Mode management fee, c6a.xlarge" ||
		dec.Lines[3].UsageType != "EKS-Auto:m5.large-management-hours") {
		t.Errorf("declared lines out of order: %+v", dec.Lines)
	}
	for _, addr := range []string{"aws_eks_cluster.outpost", "aws_eks_cluster.xl"} {
		if r := got[addr]; r.UnpricedReason() != ReasonConfig || !r.Blocking {
			t.Errorf("%s: %q %q", addr, r.UnpricedReason(), r.Unpriced())
		}
	}
}

func TestEstimatorsAndZeroCostDoNotOverlap(t *testing.T) {
	for typ := range estimators {
		if isZeroCost(typ) {
			t.Errorf("%s is both priced and zero-cost", typ)
		}
	}
}

func TestUnsupportedActionIsAnError(t *testing.T) {
	p := &plan.Plan{ResourceChanges: []plan.ResourceChange{{
		Address: "aws_eip.a", Mode: "managed", Type: "aws_eip",
		Change: plan.Change{Actions: []string{"create", "forget"}},
	}}}
	_, err := Estimate(p, &catalog.Snapshot{}, usage.Empty(), Options{Region: "us-east-1"})
	if err == nil || !strings.Contains(err.Error(), "unsupported actions") {
		t.Fatalf("err = %v", err)
	}
}

func TestForgetKeepsCost(t *testing.T) {
	data, _ := pricing.Snapshot("us-east-1")
	snap, err := catalog.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{ResourceChanges: []plan.ResourceChange{{
		Address: "aws_eip.a", Mode: "managed", Type: "aws_eip", ProviderName: "registry.terraform.io/hashicorp/aws",
		Change: plan.Change{Actions: []string{"forget"}, Before: map[string]any{"domain": "vpc"}},
	}}}
	res, err := Estimate(p, snap, usage.Empty(), Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Delta.Sign() != 0 || money.Fixed(res.After, 2) != "3.65" {
		t.Errorf("forget: delta %s after %s", money.Fixed(res.Delta, 2), money.Fixed(res.After, 2))
	}
}
