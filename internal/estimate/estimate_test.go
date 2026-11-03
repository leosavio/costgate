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

// Every fixed price key used by an estimator must exist in the embedded
// snapshot, or the gate would report a covered resource as unpriced.
func TestEmbeddedSnapshotHasEveryKey(t *testing.T) {
	data, err := pricing.Snapshot("us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := catalog.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{keyNATHours, keyNATBytes, keyIPv4, keyEndpointHours, keyEndpointBytes, keyTGWVPCHours,
		keyTGWVPCBytes, keyTGWPeerHours, keyTGWVPNHours, keyTGWVPNBytes, keyVPNHours, keyHostedZone,
		keyResolverENI, keyKMSKey, keyS3Standard, keyTrailEvents, keyPCAGeneral, keyPCAShortLived}
	for _, ut := range ebsUsageTypes {
		keys = append(keys, catalog.Key("AmazonEC2", ut, ""))
	}
	for _, it := range []string{"t3.micro", "t4g.micro", "m7g.large", "c7g.xlarge", "m5.large"} {
		keys = append(keys, instanceKey(it))
	}
	for _, k := range keys {
		if _, ok := snap.Lookup(k); !ok {
			t.Errorf("embedded snapshot lacks %s", k)
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
	_, err := Estimate(p, &catalog.Snapshot{}, usage.Empty(), "us-east-1")
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
	res, err := Estimate(p, snap, usage.Empty(), "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Delta.Sign() != 0 || money.Fixed(res.After, 2) != "3.65" {
		t.Errorf("forget: delta %s after %s", money.Fixed(res.Delta, 2), money.Fixed(res.After, 2))
	}
}
