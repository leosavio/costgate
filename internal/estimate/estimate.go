// Package estimate turns plan resource changes into monthly costs, before
// and after the change.
package estimate

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/leosavio/costgate/internal/catalog"
	"github.com/leosavio/costgate/internal/money"
	"github.com/leosavio/costgate/internal/plan"
	"github.com/leosavio/costgate/internal/usage"
)

// Billing rules: how AWS bills a line, so a consumer can tell the lines that
// should match the bill (fixed_*) from the ones whose quantity or price is a
// premise (usage).
const (
	// BillingFixedHourly: billed per hour the resource exists.
	BillingFixedHourly = "fixed_hourly"
	// BillingFixedMonthly: billed per month the resource exists (hosted
	// zone, KMS key, Private CA, provisioned EBS GB-month).
	BillingFixedMonthly = "fixed_monthly"
	// BillingUsage: depends on traffic, stored data, events or the Spot
	// market; the quantity or the price is a premise.
	BillingUsage = "usage"
)

// Line is one priced component of a resource side.
type Line struct {
	Name      string
	PriceKey  string
	UnitPrice *big.Rat
	Quantity  *big.Rat
	Unit      string
	Factor    *big.Rat // nil means 1
	Monthly   *big.Rat
	Premise   string
	Declared  bool
	Note      string
	// Billing keys, from the snapshot entry of PriceKey.
	SKU                string
	UsageType          string // with the region prefix, as in the bill
	RateCode           string // first tier, the one the estimate uses
	BillingDescription string
	BillingRule        string
}

// Reason codes of an unpriced side, stable for consumers of the JSON report.
const (
	// ReasonProvider: the resource belongs to a provider other than AWS
	// (helm, kubectl, kubernetes...).
	ReasonProvider = "provider"
	// ReasonType: AWS resource type not in the pricing map.
	ReasonType = "type"
	// ReasonRegion: costgate has no price snapshot of the resource region.
	ReasonRegion = "region"
	// ReasonConfig: a configuration the estimator does not model, or a
	// value unknown at plan time.
	ReasonConfig = "config"
	// ReasonPrice: the snapshot has no price for a key the line needs.
	ReasonPrice = "price"
)

// Side is the cost of one side (before or after) of a resource.
type Side struct {
	Exists   bool
	Monthly  *big.Rat
	Lines    []Line
	Unpriced string
	Reason   string // one of the Reason* codes when Unpriced is set
	// Region is the region whose snapshot priced the side (a replace may
	// move a resource to another region).
	Region string
}

// Resource is the estimate of one resource change.
type Resource struct {
	Address  string
	Type     string
	Action   plan.Action
	ZeroCost bool
	Before   Side
	After    Side
	Delta    *big.Rat
	// Blocking is true when the resource is unpriced on a side that the
	// change creates or modifies.
	Blocking bool
	// Region is the region of the side the report describes (after, or
	// before for a delete): the one the plan states, or Options.Region
	// when it states none (RegionAssumed).
	Region        string
	RegionAssumed bool
}

// Unpriced returns the reason the resource could not be priced, if any.
func (r Resource) Unpriced() string {
	if r.After.Unpriced != "" {
		return r.After.Unpriced
	}
	return r.Before.Unpriced
}

// UnpricedReason returns the Reason* code of Unpriced.
func (r Resource) UnpricedReason() string {
	if r.After.Unpriced != "" {
		return r.After.Reason
	}
	return r.Before.Reason
}

// Ignored is a resource left out of the estimate and of the gate because
// its provider is in Options.IgnoreProviders.
type Ignored struct {
	Address  string      `json:"address"`
	Type     string      `json:"type"`
	Provider string      `json:"provider"`
	Action   plan.Action `json:"action"`
}

// Prices returns the price snapshot of a region, or nil when costgate has
// none for it.
type Prices func(region string) (*catalog.Snapshot, error)

// How Options.Region was chosen.
const (
	// RegionFromFlag: the caller set it (--region).
	RegionFromFlag = "flag"
	// RegionFromPlan: the only region the plan names.
	RegionFromPlan = "plan"
	// RegionFromDefault: the plan names no region or several; us-east-1.
	RegionFromDefault = "default"
)

// DefaultRegion prices the resources of a plan that names no region.
const DefaultRegion = "us-east-1"

// Options control the estimate.
type Options struct {
	// Region prices the resources whose region the plan does not state;
	// every other resource is priced with the snapshot of its own region.
	Region string
	// RegionFrom is one of the RegionFrom* values.
	RegionFrom string
	// IgnoreProviders lists providers whose resources are left out of
	// the estimate and of the gate. An entry matches the provider source
	// address exactly or as its last segments: "helm", "hashicorp/helm"
	// and "registry.terraform.io/hashicorp/helm" match the Helm provider.
	IgnoreProviders []string
}

// RegionUse is a snapshot the estimate used.
type RegionUse struct {
	Region   string
	Snapshot *catalog.Snapshot
	// Resources counts the resources priced with it (types with a
	// formula, AWS provider).
	Resources int
}

// Result aggregates every managed resource of the plan.
type Result struct {
	// Region is Options.Region, and RegionFrom how it was chosen.
	Region     string
	RegionFrom string
	Resources  []Resource
	Before     *big.Rat
	After      *big.Rat
	Delta      *big.Rat
	// IgnoredProviders is Options.IgnoreProviders; Ignored lists the
	// resources it matched, in address order.
	IgnoredProviders []string
	Ignored          []Ignored
	// Snapshots lists the snapshots that priced at least one resource,
	// sorted by region; the one of Region alone when none did.
	Snapshots []RegionUse
	// Assumed counts the priced resources whose region the plan does not
	// state, priced in Region; PlanRegions are the regions it states.
	Assumed     int
	PlanRegions []string
	// RegionMismatches lists the provider configurations that deploy to a
	// region costgate has no snapshot of.
	RegionMismatches []RegionMismatch
}

// Snapshot returns the snapshot used for region, or nil.
func (r *Result) Snapshot(region string) *catalog.Snapshot {
	for _, u := range r.Snapshots {
		if u.Region == region {
			return u.Snapshot
		}
	}
	return nil
}

// PlanRegions lists the regions the plan states for its AWS resources, by
// any of the sources of the region resolver, sorted.
func PlanRegions(p *plan.Plan) []string {
	rr := newRegionResolver(p)
	seen := map[string]bool{}
	for _, rc := range p.ResourceChanges {
		if rc.Mode != "managed" || !awsProvider(rc.ProviderName) {
			continue
		}
		for _, v := range []any{rc.Change.Before, rc.Change.After} {
			if v == nil {
				continue
			}
			if r, _ := rr.resolve(rc, v); r != "" {
				seen[r] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// Blocking lists unpriced resources that the change creates, updates or replaces.
func (r *Result) Blocking() []Resource {
	var out []Resource
	for _, res := range r.Resources {
		if res.Blocking {
			out = append(out, res)
		}
	}
	return out
}

// Incomplete reports whether any resource, blocking or not, is unpriced.
func (r *Result) Incomplete() bool {
	for _, res := range r.Resources {
		if res.Unpriced() != "" {
			return true
		}
	}
	return false
}

// UndeclaredUsage lists (address, line) pairs whose quantity is a missing premise.
type UndeclaredUsage struct {
	Address string `json:"address"`
	Line    string `json:"line"`
	Premise string `json:"premise"`
}

// Undeclared returns usage-based lines without a declared premise.
func (r *Result) Undeclared() []UndeclaredUsage {
	var out []UndeclaredUsage
	for _, res := range r.Resources {
		lines := res.After.Lines
		if !res.After.Exists {
			lines = res.Before.Lines
		}
		for _, l := range lines {
			if l.Premise != "" && !l.Declared {
				out = append(out, UndeclaredUsage{res.Address, l.Name, l.Premise})
			}
		}
	}
	return out
}

func awsProvider(name string) bool {
	return strings.HasSuffix(name, "/hashicorp/aws") || name == "aws"
}

// MatchProvider reports whether the provider source address name matches
// entry: equal, or equal to its last path segments ("helm" and
// "hashicorp/helm" match "registry.terraform.io/hashicorp/helm").
func MatchProvider(name, entry string) bool {
	return name == entry || strings.HasSuffix(name, "/"+entry)
}

func ignoredProvider(name string, entries []string) bool {
	for _, e := range entries {
		if MatchProvider(name, e) {
			return true
		}
	}
	return false
}

// providerShortName is the last segment of a provider source address.
func providerShortName(name string) string {
	return name[strings.LastIndex(name, "/")+1:]
}

// Estimate prices every managed resource change of p, each side with the
// snapshot of its region. The snapshot of opt.Region must exist.
func Estimate(p *plan.Plan, prices Prices, use *usage.File, opt Options) (*Result, error) {
	region := opt.Region
	res := &Result{Region: region, RegionFrom: opt.RegionFrom, Before: money.Zero(), After: money.Zero(),
		IgnoredProviders: opt.IgnoreProviders, PlanRegions: PlanRegions(p)}
	regions := newRegionResolver(p)
	unpricedIn := map[string]int{} // region -> resources unpriced for being there
	loaded := map[string]*catalog.Snapshot{}
	used := map[string]int{}
	snapshot := func(r string) (*catalog.Snapshot, error) {
		if s, ok := loaded[r]; ok {
			return s, nil
		}
		s, err := prices(r)
		if err != nil {
			return nil, fmt.Errorf("price snapshot of %s: %w", r, err)
		}
		if s != nil && s.Region != r {
			return nil, fmt.Errorf("price snapshot of %s is for region %s", r, s.Region)
		}
		loaded[r] = s
		return s, nil
	}
	if s, err := snapshot(region); err != nil {
		return nil, err
	} else if s == nil {
		return nil, fmt.Errorf("no price snapshot for region %s", region)
	}
	// side resolves the region of one side and prices it. A side that
	// states no region takes fallback (the region the before side states),
	// then Options.Region; assumed is true in the last case.
	side := func(rc plan.ResourceChange, value, unknown any, u usage.Resolved, fallback, fallbackSrc string, zero *bool) (Side, string, string, bool, error) {
		reg, src := regions.resolve(rc, value)
		if reg == "" {
			reg, src = fallback, fallbackSrc
		}
		assumed := reg == ""
		if assumed {
			reg = region
		}
		snap, err := snapshot(reg)
		if err != nil {
			return Side{}, "", "", false, err
		}
		s := priceSide(rc, newAttrs(value, unknown), u, snap, reg, src, zero)
		s.Region = reg
		return s, reg, src, assumed, nil
	}
	for _, rc := range p.ResourceChanges {
		if rc.Mode != "managed" {
			continue
		}
		action, err := plan.Normalize(rc.Change.Actions)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rc.Address, err)
		}
		if action == plan.Read {
			continue
		}
		if ignoredProvider(rc.ProviderName, opt.IgnoreProviders) {
			res.Ignored = append(res.Ignored, Ignored{Address: rc.Address, Type: rc.Type, Provider: rc.ProviderName, Action: action})
			continue
		}
		r := Resource{Address: rc.Address, Type: rc.Type, Action: action}
		u := use.Resolve(rc.Address, rc.Type)

		beforeExists := action != plan.Create
		afterExists := action != plan.Delete
		afterValue, afterUnknown := rc.Change.After, rc.Change.AfterUnknown
		if action == plan.Forget {
			// The object stays in the account; only Terraform stops managing it.
			afterValue, afterUnknown = rc.Change.Before, nil
		}
		var beforeRegion, beforeSrc string
		if beforeExists {
			if r.Before, r.Region, beforeSrc, r.RegionAssumed, err = side(rc, rc.Change.Before, nil, u, "", "", &r.ZeroCost); err != nil {
				return nil, err
			}
			if !r.RegionAssumed {
				beforeRegion = r.Region
			}
		}
		if afterExists {
			if r.After, r.Region, _, r.RegionAssumed, err = side(rc, afterValue, afterUnknown, u, beforeRegion, beforeSrc, &r.ZeroCost); err != nil {
				return nil, err
			}
		}
		_, formula := estimators[rc.Type]
		priced := formula && awsProvider(rc.ProviderName) && !isZeroCost(rc.Type)
		switch {
		case r.UnpricedReason() == ReasonRegion:
			unpricedIn[r.Region]++
		case priced:
			used[r.Region]++
			if r.RegionAssumed {
				res.Assumed++
			}
		}
		if r.Before.Monthly == nil {
			r.Before.Monthly = money.Zero()
		}
		if r.After.Monthly == nil {
			r.After.Monthly = money.Zero()
		}
		r.Delta = money.Sub(r.After.Monthly, r.Before.Monthly)
		switch action {
		case plan.Create, plan.Update, plan.Replace:
			r.Blocking = r.After.Unpriced != ""
		}
		res.Before = money.Add(res.Before, r.Before.Monthly)
		res.After = money.Add(res.After, r.After.Monthly)
		res.Resources = append(res.Resources, r)
	}
	res.Delta = money.Sub(res.After, res.Before)
	mismatches, err := regions.mismatches(func(r string) (bool, error) {
		s, err := snapshot(r)
		return s != nil, err
	}, unpricedIn)
	if err != nil {
		return nil, err
	}
	res.RegionMismatches = mismatches
	for r, s := range loaded {
		if s != nil && used[r] > 0 {
			res.Snapshots = append(res.Snapshots, RegionUse{Region: r, Snapshot: s, Resources: used[r]})
		}
	}
	if len(res.Snapshots) == 0 {
		res.Snapshots = []RegionUse{{Region: region, Snapshot: loaded[region]}}
	}
	sort.Slice(res.Snapshots, func(i, j int) bool { return res.Snapshots[i].Region < res.Snapshots[j].Region })
	return res, nil
}

// priceSide prices one side with cat, the snapshot of resRegion (nil when
// costgate has none). resSource says where the region came from ("" when the
// plan does not state it).
func priceSide(rc plan.ResourceChange, a attrs, u usage.Resolved, cat *catalog.Snapshot, resRegion, resSource string, zero *bool) Side {
	s := Side{Exists: true, Monthly: money.Zero()}
	if isZeroCost(rc.Type) {
		*zero = true
		return s
	}
	if !awsProvider(rc.ProviderName) {
		s.Unpriced = fmt.Sprintf("provider %q is not the AWS provider; --ignore-provider %s leaves it out of the gate", rc.ProviderName, providerShortName(rc.ProviderName))
		s.Reason = ReasonProvider
		return s
	}
	est, ok := estimators[rc.Type]
	if !ok {
		s.Unpriced, s.Reason = "resource type not in the pricing map", ReasonType
		return s
	}
	if cat == nil {
		s.Reason = ReasonRegion
		switch resSource {
		case regionFromAttribute, "":
			s.Unpriced = fmt.Sprintf("no price snapshot for resource region %s", resRegion)
		default:
			s.Unpriced = fmt.Sprintf("no price snapshot for resource region %s (from %s)", resRegion, resSource)
		}
		return s
	}
	out := est(a, u)
	if out.unpriced != "" {
		s.Unpriced, s.Reason = out.unpriced, ReasonConfig
		return s
	}
	if len(out.comps) == 0 {
		*zero = true
	}
	for _, c := range out.comps {
		if c.key == "" && c.premise != "" && !c.declared {
			// A premise-only line: the quantity is not declared and the
			// SKU depends on it (EKS Auto Mode instance type). Counted 0.
			// Any other comp without a key falls through to the lookup
			// below and fails closed.
			s.Lines = append(s.Lines, Line{
				Name: c.name, UnitPrice: money.Zero(), Quantity: c.qty, Unit: c.unit, Monthly: money.Zero(),
				Premise: c.premise, Declared: false, Note: c.note, BillingRule: c.rule,
			})
			continue
		}
		price, ok := cat.Lookup(c.key)
		if !ok {
			s.Unpriced, s.Reason = fmt.Sprintf("no price for %s in the snapshot", c.key), ReasonPrice
			s.Lines, s.Monthly = nil, money.Zero()
			return s
		}
		factors := []*big.Rat{price, c.qty}
		if c.factor != nil {
			factors = append(factors, c.factor)
		}
		m := money.Mul(factors...)
		info := cat.Info(c.key)
		s.Lines = append(s.Lines, Line{
			Name: c.name, PriceKey: c.key, UnitPrice: price, Quantity: c.qty, Unit: c.unit,
			Factor: c.factor, Monthly: m, Premise: c.premise, Declared: c.declared, Note: c.note,
			SKU: info.SKU, UsageType: info.UsageType, RateCode: info.RateCode,
			BillingDescription: info.Description, BillingRule: c.rule,
		})
		s.Monthly = money.Add(s.Monthly, m)
	}
	return s
}
