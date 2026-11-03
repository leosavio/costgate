// Package estimate turns plan resource changes into monthly costs, before
// and after the change.
package estimate

import (
	"fmt"
	"math/big"
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

// Side is the cost of one side (before or after) of a resource.
type Side struct {
	Exists   bool
	Monthly  *big.Rat
	Lines    []Line
	Unpriced string
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
}

// Unpriced returns the reason the resource could not be priced, if any.
func (r Resource) Unpriced() string {
	if r.After.Unpriced != "" {
		return r.After.Unpriced
	}
	return r.Before.Unpriced
}

// Result aggregates every managed resource of the plan.
type Result struct {
	Region    string
	Resources []Resource
	Before    *big.Rat
	After     *big.Rat
	Delta     *big.Rat
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

// Estimate prices every managed resource change of p.
func Estimate(p *plan.Plan, cat *catalog.Snapshot, use *usage.File, region string) (*Result, error) {
	res := &Result{Region: region, Before: money.Zero(), After: money.Zero()}
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
		r := Resource{Address: rc.Address, Type: rc.Type, Action: action}
		u := use.Resolve(rc.Address, rc.Type)

		beforeExists := action != plan.Create
		afterExists := action != plan.Delete
		afterValue, afterUnknown := rc.Change.After, rc.Change.AfterUnknown
		if action == plan.Forget {
			// The object stays in the account; only Terraform stops managing it.
			afterValue, afterUnknown = rc.Change.Before, nil
		}
		if beforeExists {
			r.Before = priceSide(rc, newAttrs(rc.Change.Before, nil), u, cat, region, &r.ZeroCost)
		}
		if afterExists {
			r.After = priceSide(rc, newAttrs(afterValue, afterUnknown), u, cat, region, &r.ZeroCost)
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
	return res, nil
}

func priceSide(rc plan.ResourceChange, a attrs, u usage.Resolved, cat *catalog.Snapshot, region string, zero *bool) Side {
	s := Side{Exists: true, Monthly: money.Zero()}
	if isZeroCost(rc.Type) {
		*zero = true
		return s
	}
	est, ok := estimators[rc.Type]
	if !ok {
		s.Unpriced = "resource type not in the pricing map"
		return s
	}
	if !awsProvider(rc.ProviderName) {
		s.Unpriced = fmt.Sprintf("provider %q is not the AWS provider", rc.ProviderName)
		return s
	}
	if rr, ok := a.str("region"); ok && rr != "" && rr != region {
		s.Unpriced = fmt.Sprintf("resource region %s differs from priced region %s", rr, region)
		return s
	}
	out := est(a, u)
	if out.unpriced != "" {
		s.Unpriced = out.unpriced
		return s
	}
	if len(out.comps) == 0 {
		*zero = true
	}
	for _, c := range out.comps {
		price, ok := cat.Lookup(c.key)
		if !ok {
			s.Unpriced = fmt.Sprintf("no price for %s in the snapshot", c.key)
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
