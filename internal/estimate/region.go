package estimate

import (
	"regexp"
	"sort"
	"strings"

	"github.com/leosavio/costgate/internal/plan"
)

// The plan carries the region of a resource in different places, depending
// on the AWS provider version: 6.x adds a region attribute to every
// resource; 5.x keeps it in the provider configuration (in the plan only
// when it is a constant) and in the ARNs the provider returns. Sources, in
// the order they are tried:
const (
	regionFromAttribute = "region attribute"
	regionFromProvider  = "provider configuration"
	regionFromARN       = "ARN"
	regionFromSiblings  = "ARNs of other resources of provider configuration"
)

// RegionMismatch groups the provider configurations of the plan that deploy
// to a region costgate has no price snapshot of.
type RegionMismatch struct {
	// Region is where the provider configurations deploy.
	Region string
	// Providers are the provider_config_key values, sorted.
	Providers []string
	// Inferred is true when no provider configuration states the region (a
	// constant or a root variable) and it comes from the region attribute
	// or ARNs of its resources.
	Inferred bool
	// Resources counts the resources left unpriced because they are in
	// Region (resources with no charge are not counted).
	Resources int
}

var regionPattern = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)

// arnRegion returns the region field of an ARN
// (arn:<partition>:<service>:<region>:<account>:<resource>), or "" when s is
// not an ARN or the service is global (empty region field).
func arnRegion(s string) string {
	if !strings.HasPrefix(s, "arn:") {
		return ""
	}
	parts := strings.SplitN(s, ":", 6)
	if len(parts) < 6 || !regionPattern.MatchString(parts[3]) {
		return ""
	}
	return parts[3]
}

// ownARNRegion returns the region of the resource's own ARN: the arn
// attribute, then an id that is an ARN, then the ARNs nested in the values
// when they all name the same region. A resource may reference an ARN in
// another region (a KMS replica key names its primary key), so conflicting
// nested ARNs give no answer.
func ownARNRegion(v any) string {
	m, _ := v.(map[string]any)
	for _, k := range []string{"arn", "id"} {
		if s, ok := m[k].(string); ok {
			if r := arnRegion(s); r != "" {
				return r
			}
		}
	}
	found, conflict := "", false
	walkStrings(v, func(s string) {
		r := arnRegion(s)
		switch {
		case r == "":
		case found == "":
			found = r
		case found != r:
			conflict = true
		}
	})
	if conflict {
		return ""
	}
	return found
}

func walkStrings(v any, f func(string)) {
	switch x := v.(type) {
	case string:
		f(x)
	case map[string]any:
		for _, e := range x {
			walkStrings(e, f)
		}
	case []any:
		for _, e := range x {
			walkStrings(e, f)
		}
	}
}

// directRegion is the region a side states by itself: the region attribute
// or its ARNs.
func directRegion(v any) (string, string) {
	m, _ := v.(map[string]any)
	if s, _ := m["region"].(string); s != "" {
		return s, regionFromAttribute
	}
	if s := ownARNRegion(v); s != "" {
		return s, regionFromARN
	}
	return "", ""
}

// regionResolver finds the region of each side of a resource change.
type regionResolver struct {
	plan *plan.Plan
	// learned maps a provider_config_key to the one region that the
	// region attributes and ARNs of its resources name; absent when they
	// name none or more than one.
	learned map[string]string
	// keys are the provider configurations used by AWS resource changes.
	keys map[string]bool
}

func newRegionResolver(p *plan.Plan) *regionResolver {
	seen := map[string]map[string]bool{}
	keys := map[string]bool{}
	for _, rc := range p.ResourceChanges {
		if rc.Mode != "managed" || !awsProvider(rc.ProviderName) {
			continue
		}
		key := p.ProviderConfigKey(rc.Address)
		if key == "" {
			continue
		}
		keys[key] = true
		for _, v := range []any{rc.Change.Before, rc.Change.After} {
			r, _ := directRegion(v)
			if r == "" {
				continue
			}
			if seen[key] == nil {
				seen[key] = map[string]bool{}
			}
			seen[key][r] = true
		}
	}
	learned := map[string]string{}
	for key, regions := range seen {
		if len(regions) == 1 {
			for r := range regions {
				learned[key] = r
			}
		}
	}
	return &regionResolver{plan: p, learned: learned, keys: keys}
}

// resolve returns the region of one side and where it came from: the
// region attribute, the constant region of the provider configuration, the
// ARNs of the side, or the region every other resource of the same provider
// configuration names. "" when none applies.
func (rr *regionResolver) resolve(rc plan.ResourceChange, value any) (string, string) {
	m, _ := value.(map[string]any)
	if s, _ := m["region"].(string); s != "" {
		return s, regionFromAttribute
	}
	key := rr.plan.ProviderConfigKey(rc.Address)
	if r, ok := rr.plan.ProviderRegion(key); ok {
		return r, regionFromProvider + " " + key
	}
	if s := ownARNRegion(value); s != "" {
		return s, regionFromARN
	}
	if s := rr.learned[key]; s != "" {
		return s, regionFromSiblings + " " + key
	}
	return "", ""
}

// mismatches lists the provider configurations whose region (stated, or
// learned from their resources) has no price snapshot. found counts, by
// region, the resources left unpriced for being there.
func (rr *regionResolver) mismatches(has func(region string) (bool, error), found map[string]int) ([]RegionMismatch, error) {
	byRegion := map[string]*RegionMismatch{}
	for key := range rr.keys {
		r, constant := rr.plan.ProviderRegion(key)
		if !constant {
			r = rr.learned[key]
		}
		if r == "" {
			continue
		}
		ok, err := has(r)
		if err != nil {
			return nil, err
		}
		if ok {
			continue
		}
		m := byRegion[r]
		if m == nil {
			m = &RegionMismatch{Region: r, Inferred: true, Resources: found[r]}
			byRegion[r] = m
		}
		m.Providers = append(m.Providers, key)
		if constant {
			m.Inferred = false
		}
	}
	out := make([]RegionMismatch, 0, len(byRegion))
	for _, m := range byRegion {
		sort.Strings(m.Providers)
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Region < out[j].Region })
	return out, nil
}
