package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	snap, err := Refresh(context.Background(), fixture(t), "us-east-1", Rules)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		Key("AmazonEC2", "BoxUsage:t3.micro", "RunInstances"):         "0.0104000000",
		Key("AmazonEC2", "BoxUsage:t4g.micro", "RunInstances"):        "0.0084000000",
		Key("AmazonEC2", "NatGateway-Hours", "NatGateway"):            "0.0450000000",
		Key("AmazonVPC", "PublicIPv4:InUseAddress", ""):               "0.0050000000",
		Key("AmazonVPC", "TransitGateway-Hours", "TransitGatewayVPC"): "0.0500000000",
		Key("AmazonRoute53", "HostedZone", ""):                        "0.5000000000",
		Key("awskms", "KMS-Keys", ""):                                 "1.0000000000",
		Key("AmazonS3", "TimedStorage-ByteHrs", ""):                   "0.0230000000",
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
	if snap.AsOf != "2026-11-03T00:09:47Z" {
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
	_, err := Refresh(context.Background(), fixture(t), "us-east-1", rules)
	if err == nil || !strings.Contains(err.Error(), "matched no on-demand price") {
		t.Fatalf("err = %v", err)
	}
}

func TestExtractFailsOnAmbiguousRule(t *testing.T) {
	doc := `"FormatVersion","v1.0"
"Disclaimer","x"
"Publication Date","2026-01-01T00:00:00Z"
"Version","20260101000000"
"OfferCode","AmazonEC2"
"SKU","TermType","PriceDescription","StartingRange","EndingRange","Unit","PricePerUnit","Currency","usageType","operation","Region Code","Operating System"
"A","OnDemand","linux","0","Inf","Hrs","0.0104","USD","BoxUsage:t3.micro","RunInstances","us-east-1","Linux"
"B","OnDemand","other","0","Inf","Hrs","0.0200","USD","BoxUsage:t3.micro","RunInstances","us-east-1","Other"
`
	rule := Rule{Service: "AmazonEC2", Scope: ScopeRegion, UsageType: "BoxUsage:", UsagePrefix: true, Operation: "RunInstances"}
	_, _, err := extract(strings.NewReader(doc), fileKey{"AmazonEC2", ScopeRegion}, "us-east-1", regionPrefixes["us-east-1"], []Rule{rule})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
	rule.Match = map[string]string{"Operating System": "Linux"}
	_, prices, err := extract(strings.NewReader(doc), fileKey{"AmazonEC2", ScopeRegion}, "us-east-1", regionPrefixes["us-east-1"], []Rule{rule})
	if err != nil || prices[Key("AmazonEC2", "BoxUsage:t3.micro", "RunInstances")].SKU != "A" {
		t.Fatalf("with filter: %v %v", prices, err)
	}
}

func TestLoadRejectsTamperedAndNewerSnapshots(t *testing.T) {
	snap, err := Refresh(context.Background(), fixture(t), "us-east-1", Rules)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := snap.Encode()
	tampered := strings.Replace(string(data), `"usd": "0.0450000000"`, `"usd": "0.0010000000"`, 1)
	if _, err := Load([]byte(tampered)); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("tampered: err = %v", err)
	}
	newer := strings.Replace(string(data), `"schema_version": 1`, `"schema_version": 2`, 1)
	if _, err := Load([]byte(newer)); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("newer: err = %v", err)
	}
}

func TestNormalizeUsageType(t *testing.T) {
	p := regionPrefixes["us-east-1"]
	cases := map[string]string{
		"USE1-VpcEndpoint-Hours": "VpcEndpoint-Hours",
		"us-east-1-KMS-Keys":     "KMS-Keys",
		"NatGateway-Hours":       "NatGateway-Hours",
		"USE2-NatGateway-Hours":  "USE2-NatGateway-Hours",
	}
	for in, want := range cases {
		if got := normalizeUsageType(in, p); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}
