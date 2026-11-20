package plan

import (
	"strings"
	"testing"
)

func TestConfigAddress(t *testing.T) {
	cases := map[string]string{
		"aws_eip.nat":               "aws_eip.nat",
		"module.vpc.aws_eip.nat[0]": "module.vpc.aws_eip.nat",
		`module.eks["a"].module.kms.aws_kms_key.this[0]`: "module.eks.module.kms.aws_kms_key.this",
		`aws_route53_zone.this["ex]ample.com"]`:          "aws_route53_zone.this",
		`aws_s3_bucket.b["a\"]b"]`:                       "aws_s3_bucket.b",
	}
	for in, want := range cases {
		if got := ConfigAddress(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func TestProviderConfigFromConfiguration(t *testing.T) {
	doc := `{"format_version": "1.2", "resource_changes": [],
	  "configuration": {
	    "provider_config": {
	      "aws": {"name": "aws", "full_name": "registry.terraform.io/hashicorp/aws", "expressions": {"region": {"constant_value": "us-west-2"}}},
	      "aws.dr": {"name": "aws", "alias": "dr", "expressions": {"region": {"references": ["var.dr_region"]}}},
	      "module.legacy:aws": {"name": "aws", "module_address": "module.legacy", "expressions": {"region": {"constant_value": "eu-west-1"}}}
	    },
	    "root_module": {
	      "resources": [{"address": "aws_eip.a", "mode": "managed", "provider_config_key": "aws"}],
	      "module_calls": {
	        "net": {"module": {
	          "resources": [{"address": "aws_nat_gateway.this", "mode": "managed", "provider_config_key": "aws.dr"}],
	          "module_calls": {"inner": {"module": {"resources": [{"address": "aws_eip.b", "mode": "managed", "provider_config_key": "aws"}]}}}
	        }},
	        "legacy": {"module": {"resources": [{"address": "aws_kms_key.k", "mode": "managed", "provider_config_key": "module.legacy:aws"}]}}
	      }
	    }
	  }}`
	p, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ address, key, region string }{
		{"aws_eip.a", "aws", "us-west-2"},
		{`module.net["x"].aws_nat_gateway.this`, "aws.dr", ""},
		{"module.net.module.inner.aws_eip.b[0]", "aws", "us-west-2"},
		{"module.legacy.aws_kms_key.k", "module.legacy:aws", "eu-west-1"},
		{"aws_eip.not_in_config", "", ""},
	}
	for _, c := range cases {
		key := p.ProviderConfigKey(c.address)
		region, ok := p.ProviderRegion(key)
		if key != c.key || region != c.region || ok != (c.region != "") {
			t.Errorf("%s: key %q region %q (%v), want %q %q", c.address, key, region, ok, c.key, c.region)
		}
	}
}

func TestProviderRegionFromRootVariable(t *testing.T) {
	doc := `{"format_version": "1.2", "resource_changes": [],
	  "variables": {"region": {"value": "sa-east-1"}, "tags": {"value": {"a": "b"}}},
	  "configuration": {
	    "provider_config": {
	      "aws": {"name": "aws", "expressions": {"region": {"references": ["var.region"]}}},
	      "aws.map": {"name": "aws", "alias": "map", "expressions": {"region": {"references": ["var.tags[\"a\"]", "var.tags"]}}},
	      "aws.obj": {"name": "aws", "alias": "obj", "expressions": {"region": {"references": ["var.tags"]}}},
	      "aws.local": {"name": "aws", "alias": "local", "expressions": {"region": {"references": ["local.region"]}}},
	      "aws.unset": {"name": "aws", "alias": "unset", "expressions": {"region": {"references": ["var.missing"]}}},
	      "module.m:aws": {"name": "aws", "module_address": "module.m", "expressions": {"region": {"references": ["var.region"]}}}
	    },
	    "root_module": {}
	  }}`
	p, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"aws": "sa-east-1", "aws.map": "", "aws.obj": "", "aws.local": "", "aws.unset": "", "module.m:aws": ""}
	for key, region := range want {
		if got, ok := p.ProviderRegion(key); got != region || ok != (region != "") {
			t.Errorf("%s: %q %v, want %q", key, got, ok, region)
		}
	}
}
