package usage

import (
	"os"
	"strings"
	"testing"

	"github.com/leosavio/costgate/internal/money"
)

func TestResolvePrecedence(t *testing.T) {
	f, err := Load([]byte(`{
		"schema_version": 1,
		"defaults": {"hours_per_month": 700, "spot_discount": 0.5},
		"types": {"aws_nat_gateway": {"gb_per_month": 10, "hours_per_month": 600}},
		"resources": {
			"module.net.aws_nat_gateway.this": {"gb_per_month": 20},
			"module.net.aws_nat_gateway.this[\"b\"]": {"gb_per_month": 30}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	check := func(addr string, wantGB, wantHours string) {
		t.Helper()
		r := f.Resolve(addr, "aws_nat_gateway")
		if money.Trim(r.GB.V) != wantGB || money.Trim(r.Hours.V) != wantHours {
			t.Errorf("%s: gb=%s (%s) hours=%s (%s), want %s/%s", addr, money.Trim(r.GB.V), r.GB.From, money.Trim(r.Hours.V), r.Hours.From, wantGB, wantHours)
		}
	}
	check(`module.net.aws_nat_gateway.this["b"]`, "30", "600")
	check(`module.net.aws_nat_gateway.this["a"]`, "20", "600")
	check(`aws_nat_gateway.other`, "10", "600")

	r := f.Resolve("aws_instance.x", "aws_instance")
	if money.Trim(r.Hours.V) != "700" || money.Trim(r.SpotDiscount.V) != "0.5" || r.GB.Declared {
		t.Errorf("defaults not applied: %+v", r)
	}
	if money.Trim(Empty().Resolve("a.b", "a").Hours.V) != "730" {
		t.Error("built-in 730 h not applied")
	}
}

func TestLoadValidation(t *testing.T) {
	bad := map[string]string{
		"version":       `{"schema_version": 2}`,
		"missing":       `{}`,
		"unknown field": `{"schema_version": 1, "defaults": {"gbs": 1}}`,
		"hours":         `{"schema_version": 1, "defaults": {"hours_per_month": 800}}`,
		"spot 1":        `{"schema_version": 1, "defaults": {"spot_discount": 1}}`,
		"negative gb":   `{"schema_version": 1, "types": {"aws_s3_bucket": {"gb_per_month": -1}}}`,
		"az fraction":   `{"schema_version": 1, "defaults": {"az_count": 1.5}}`,
	}
	for name, doc := range bad {
		if _, err := Load([]byte(doc)); err == nil {
			t.Errorf("%s: accepted %s", name, doc)
		} else if !strings.HasPrefix(err.Error(), "usage file") {
			t.Errorf("%s: error without context: %v", name, err)
		}
	}
}

func TestExampleUsageFileLoads(t *testing.T) {
	data, err := os.ReadFile("../../examples/usage.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(data); err != nil {
		t.Fatal(err)
	}
}
