package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) Fetcher {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir(filepath.Join("..", "..", "testdata", "offers"))))
	t.Cleanup(srv.Close)
	return Fetcher{BaseURL: srv.URL, Client: srv.Client()}
}

func TestRefreshFiltersOnDemandLinuxShared(t *testing.T) {
	snap, _, err := Refresh(context.Background(), fixture(t), "us-east-1", Rules, true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		Key("AmazonEC2", "BoxUsage:t3.micro", "RunInstances"):                                   "0.0104000000",
		Key("AmazonEC2", "BoxUsage:t4g.micro", "RunInstances"):                                  "0.0084000000",
		Key("AmazonEC2", "NatGateway-Hours", "NatGateway"):                                      "0.0450000000",
		Key("AmazonVPC", "PublicIPv4:InUseAddress", ""):                                         "0.0050000000",
		Key("AmazonVPC", "TransitGateway-Hours", "TransitGatewayVPC"):                           "0.0500000000",
		Key("AmazonRoute53", "HostedZone", ""):                                                  "0.5000000000",
		Key("awskms", "KMS-Keys", ""):                                                           "1.0000000000",
		Key("AmazonS3", "TimedStorage-ByteHrs", ""):                                             "0.0230000000",
		Key("AmazonEC2", "EBS:VolumeP-IOPS.gp3", ""):                                            "0.0050000000",
		Key("AmazonEC2", "EBS:VolumeP-Throughput.gp3", ""):                                      "40.9600000000",
		Key("AmazonEKS", "AmazonEKS-Hours:perCluster", "CreateOperation"):                       "0.1000000000",
		Key("AmazonEKS", "AmazonEKS-Hours:extendedSupport", "ExtendedSupport"):                  "0.5000000000",
		Key("AmazonEKS", "EKS-Auto:c6a.xlarge-management-hours", "EKSAutoUsage"):                "0.0183600000",
		Key("AmazonEKS", "AmazonEKS-Hours:XL-ProvisionedTier", "ProvisionedControlPlaneUsage"):  "1.6500000000",
		Key("AmazonEKS", "AmazonEKS-Hours:8XL-ProvisionedTier", "ProvisionedControlPlaneUsage"): "13.9000000000",
		Key("AWSSecretsManager", "AWSSecretsManager-Secrets", ""):                               "0.4000000000",
	}
	for k, usd := range want {
		if got := snap.Prices[k].USD; got != usd {
			t.Errorf("%s = %q, want %q", k, got, usd)
		}
	}
	for k := range snap.Prices {
		if strings.Contains(k, "BoxUsage") && !strings.HasSuffix(k, "|RunInstances") {
			t.Errorf("non-Linux price leaked: %s", k)
		}
	}
	if snap.Prices[Key("AmazonRoute53", "HostedZone", "")].TierEnd != "25" {
		t.Error("hosted zone first tier end not recorded")
	}
	for k := range snap.Prices {
		if strings.HasPrefix(k, "AmazonEKS|") && strings.Contains(k, "Outposts") || strings.Contains(k, "Fargate") {
			t.Errorf("EKS price outside the rules leaked: %s", k)
		}
	}
	if eks := snap.Prices[Key("AmazonEKS", "AmazonEKS-Hours:perCluster", "CreateOperation")]; eks.UsageType != "USE1-AmazonEKS-Hours:perCluster" {
		t.Errorf("eks usage type %q", eks.UsageType)
	}
	// The newest offer file of the mirror is AWSSecretsManager 20261111195606.
	if snap.AsOf != "2026-11-11T19:56:06Z" {
		t.Errorf("as_of = %s", snap.AsOf)
	}
	data, err := snap.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(data); err != nil {
		t.Fatalf("round trip: %v", err)
	}
}

func TestRefreshFailsWhenRuleMatchesNothing(t *testing.T) {
	rules := append([]Rule{}, Rules...)
	rules = append(rules, Rule{Service: "AmazonVPC", Scope: ScopeRegion, UsageType: "Renamed-Hours", Operation: ""})
	_, _, err := Refresh(context.Background(), fixture(t), "us-east-1", rules, true)
	if err == nil || !strings.Contains(err.Error(), "matched no on-demand price") {
		t.Fatalf("err = %v", err)
	}
	// Not strict: the snapshot is written without the rule, which is listed.
	snap, missing, err := Refresh(context.Background(), fixture(t), "us-east-1", rules, false)
	if err != nil || len(missing) != 1 || !strings.Contains(missing[0], "Renamed-Hours") || len(snap.Prices) == 0 {
		t.Fatalf("lenient: %v %q", err, missing)
	}
}

func TestRegionsHavePrefixes(t *testing.T) {
	regions := Regions()
	if len(regions) != 36 || regions[0] != "af-south-1" || regions[len(regions)-1] != "us-west-2" {
		t.Fatalf("regions %d: %v", len(regions), regions)
	}
	seen := map[string]string{}
	for _, r := range regions {
		p, ok := regionPrefixes(r)
		if !ok || len(p) != 2 || p[1] != r+"-" {
			t.Errorf("%s: prefixes %q", r, p)
		}
		if prev, dup := seen[p[0]]; dup {
			t.Errorf("prefix %s used by %s and %s", p[0], prev, r)
		}
		seen[p[0]] = r
	}
	if p, _ := regionPrefixes("eu-west-1"); p[0] != "EU-" {
		t.Errorf("eu-west-1 prefix %q", p[0])
	}
	if RegionSupported("cn-north-1") || RegionSupported("us-east-1-bos-1") {
		t.Error("China and Local Zones are not Price List regions of this endpoint")
	}
}

func TestExtractFailsOnAmbiguousRule(t *testing.T) {
	doc := `"FormatVersion","v1.0"
"Disclaimer","x"
"Publication Date","2026-01-01T00:00:00Z"
"Version","20260101000000"
"OfferCode","AmazonEC2"
"SKU","TermType","PriceDescription","StartingRange","EndingRange","Unit","PricePerUnit","Currency","usageType","operation","Region Code","Operating System","RateCode","serviceName","Location"
"A","OnDemand","linux","0","Inf","Hrs","0.0104","USD","BoxUsage:t3.micro","RunInstances","us-east-1","Linux","A.1.1","EC2","US East (N. Virginia)"
"B","OnDemand","other","0","Inf","Hrs","0.0200","USD","BoxUsage:t3.micro","RunInstances","us-east-1","Other","B.1.1","EC2","US East (N. Virginia)"
`
	rule := Rule{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "BoxUsage:", UsagePrefix: true, Operation: "RunInstances"}
	p, _ := regionPrefixes("us-east-1")
	var missing []string
	_, _, err := extract(strings.NewReader(doc), fileKey{"AmazonEC2", ScopeRegion}, "us-east-1", p, []Rule{rule}, true, &missing)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
	rule.Match = map[string]string{"Operating System": "Linux"}
	_, prices, err := extract(strings.NewReader(doc), fileKey{"AmazonEC2", ScopeRegion}, "us-east-1", p, []Rule{rule}, true, &missing)
	if err != nil || prices[Key("AmazonEC2", "BoxUsage:t3.micro", "RunInstances")].SKU != "A" {
		t.Fatalf("with filter: %v %v", prices, err)
	}
}

func TestLoadRejectsTamperedAndNewerSnapshots(t *testing.T) {
	snap, _, err := Refresh(context.Background(), fixture(t), "us-east-1", Rules, true)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := snap.Encode()
	tampered := strings.Replace(string(data), `"usd": "0.0450000000"`, `"usd": "0.0010000000"`, 1)
	if _, err := Load([]byte(tampered)); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("tampered: err = %v", err)
	}
	newer := strings.Replace(string(data), `"schema_version": 2`, `"schema_version": 3`, 1)
	if _, err := Load([]byte(newer)); err == nil || !strings.Contains(err.Error(), "schema_version 3 not supported") {
		t.Errorf("newer: err = %v", err)
	}
	// A newer snapshot also carries fields this build does not know; the
	// error must still name the version.
	newerField := strings.Replace(newer, `"region": "us-east-1"`, `"region": "us-east-1", "v3_field": 1`, 1)
	if _, err := Load([]byte(newerField)); err == nil || !strings.Contains(err.Error(), "schema_version 3 not supported") {
		t.Errorf("newer with unknown field: err = %v", err)
	}
	older := strings.Replace(string(data), `"schema_version": 2`, `"schema_version": 0`, 1)
	if _, err := Load([]byte(older)); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("older: err = %v", err)
	}
	unknown := strings.Replace(string(data), `"region": "us-east-1"`, `"region": "us-east-1", "extra": 1`, 1)
	if _, err := Load([]byte(unknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("unknown field: err = %v", err)
	}
}

// A schema_version 1 snapshot (costgate 0.1.x) has no v2 field, and its
// digest is unchanged because every v2 field is omitempty.
func TestLoadAcceptsV1Snapshot(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pricing.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if snap.SchemaVersion != 1 {
		t.Fatalf("schema_version %d", snap.SchemaVersion)
	}
	p := snap.Info(Key("AmazonRoute53", "HostedZone", ""))
	if p.SKU == "" || p.RateCode != "" || p.Tiers != nil {
		t.Errorf("v1 hosted zone: %+v", p)
	}
}

func TestRefreshKeepsBillingFieldsAndTiers(t *testing.T) {
	snap, _, err := Refresh(context.Background(), fixture(t), "us-east-1", Rules, true)
	if err != nil {
		t.Fatal(err)
	}
	if snap.SchemaVersion != 2 {
		t.Errorf("schema_version %d", snap.SchemaVersion)
	}
	zone := snap.Prices[Key("AmazonRoute53", "HostedZone", "")]
	want := Price{
		SKU: "KVPGEJE88UW8S779", Unit: "HostedZone", USD: "0.5000000000", TierEnd: "25",
		Description: "$0.50 per Hosted Zone for the first 25 Hosted Zones",
		RateCode:    "KVPGEJE88UW8S779.JRTCKXETXF.G2A93NZ7DT", UsageType: "HostedZone",
		ServiceName: "Amazon Route 53", Location: "Any",
	}
	if !sameFirstTier(zone, want) {
		t.Errorf("hosted zone first tier:\n got %+v\nwant %+v", zone, want)
	}
	if len(zone.Tiers) != 1 || zone.Tiers[0] != (Tier{Start: "25", USD: "0.1000000000",
		RateCode: "KVPGEJE88UW8S779.JRTCKXETXF.VYAP54KATQ", Description: "$0.10 per Hosted Zone for additional Hosted Zones"}) {
		t.Errorf("hosted zone tiers: %+v", zone.Tiers)
	}
	ip := snap.Prices[Key("AmazonVPC", "PublicIPv4:InUseAddress", "")]
	if ip.UsageType != "USE1-PublicIPv4:InUseAddress" || ip.Location != "US East (N. Virginia)" || ip.RateCode == "" {
		t.Errorf("public IPv4: %+v", ip)
	}
	if kms := snap.Prices[Key("awskms", "KMS-Keys", "")]; kms.UsageType != "us-east-1-KMS-Keys" {
		t.Errorf("kms usage type %q", kms.UsageType)
	}
	s3 := snap.Prices[Key("AmazonS3", "TimedStorage-ByteHrs", "")]
	for i := 1; i < len(s3.Tiers); i++ {
		if s3.Tiers[i-1].End != s3.Tiers[i].Start {
			t.Errorf("s3 tiers not contiguous: %+v", s3.Tiers)
		}
	}
	if len(s3.Tiers) > 0 && s3.TierEnd != s3.Tiers[0].Start {
		t.Errorf("s3 first tier ends at %s, next starts at %s", s3.TierEnd, s3.Tiers[0].Start)
	}
}

func TestNormalizeUsageType(t *testing.T) {
	p, _ := regionPrefixes("us-east-1")
	cases := map[string]string{
		"USE1-VpcEndpoint-Hours":         "VpcEndpoint-Hours",
		"USE1-AWSSecretsManager-Secret":  "AWSSecretsManager-Secrets",
		"USE1-AWSSecretsManager-Secrets": "AWSSecretsManager-Secrets",
		"us-east-1-KMS-Keys":             "KMS-Keys",
		"NatGateway-Hours":               "NatGateway-Hours",
		"USE2-NatGateway-Hours":          "USE2-NatGateway-Hours",
	}
	for in, want := range cases {
		if got := normalizeUsageType(in, p); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}
