package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/leosavio/costgate/internal/catalog"
	"github.com/leosavio/costgate/internal/estimate"
	"github.com/leosavio/costgate/internal/money"
	"github.com/leosavio/costgate/internal/plan"
)

// Marker is the first line of the Markdown report, so a CI job can find and
// update its previous comment instead of adding a new one.
const Marker = "<!-- costgate -->"

// JSONSchemaVersion is the version of the JSON report.
const JSONSchemaVersion = 1

// Input groups what the renderers need.
type Input struct {
	Version  string
	Result   *estimate.Result
	Verdict  Verdict
	Ceilings Ceilings
	Pricing  *catalog.Snapshot
}

func usd(r *big.Rat) string { return money.Fixed(r, 2) }

// signedUSD renders "+US$ 12.30", "-US$ 4.00" or "US$ 0.00".
func signedUSD(r *big.Rat) string {
	s := money.Signed(r, 2)
	if s[0] == '+' || s[0] == '-' {
		return s[:1] + "US$ " + s[1:]
	}
	return "US$ " + s
}

func cell(s *estimate.Side) string {
	switch {
	case !s.Exists:
		return "–"
	case s.Unpriced != "":
		return "?"
	}
	return usd(s.Monthly)
}

func lineText(l estimate.Line) string {
	if l.Premise != "" && !l.Declared {
		return fmt.Sprintf("%s: usage not declared (`%s`)", l.Name, l.Premise)
	}
	parts := []string{money.Trim(l.Quantity) + " " + l.Unit, "$" + money.Trim(l.UnitPrice)}
	if l.Factor != nil {
		parts = append(parts, money.Trim(l.Factor))
	}
	s := fmt.Sprintf("%s: %s = %s", l.Name, strings.Join(parts, " × "), usd(l.Monthly))
	if l.Note != "" {
		s += " (" + l.Note + ")"
	}
	return s
}

func breakdown(r estimate.Resource) string {
	s := &r.After
	if !s.Exists {
		s = &r.Before
	}
	if s.Unpriced != "" {
		return "unpriced: " + s.Unpriced
	}
	if r.ZeroCost && len(s.Lines) == 0 {
		return "no charge"
	}
	var out []string
	for _, l := range s.Lines {
		out = append(out, lineText(l))
	}
	return strings.Join(out, "<br>")
}

func escape(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// Markdown renders the pull request comment.
func Markdown(in Input) string {
	res, v := in.Result, in.Verdict
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	icon := map[string]string{StatusOK: "✅", StatusOverCeiling: "❌", StatusUnpriced: "⚠️"}[v.Status]
	w("%s\n### costgate: %s %s/month%s\n\n", Marker, icon, signedUSD(res.Delta), statusText(v.Status))
	w("| Monthly cost (USD) | Before | After | Delta |\n|---|---:|---:|---:|\n")
	w("| Total | %s | %s | **%s** |\n\n", usd(res.Before), usd(res.After), money.Signed(res.Delta, 2))

	w("| Check | Limit | Result |\n|---|---:|---|\n")
	if in.Ceilings.MaxDelta != nil {
		ok := res.Delta.Cmp(in.Ceilings.MaxDelta) <= 0
		w("| `--max-monthly-delta` | %s | %s |\n", usd(in.Ceilings.MaxDelta), pass(ok, money.Signed(res.Delta, 2)))
	}
	if in.Ceilings.MaxTotal != nil {
		ok := res.After.Cmp(in.Ceilings.MaxTotal) <= 0 && !res.Incomplete()
		w("| `--max-monthly-total` | %s | %s |\n", usd(in.Ceilings.MaxTotal), pass(ok, usd(res.After)))
	}
	blocking := res.Blocking()
	w("| Unpriced resources created or changed | 0 | %s |\n\n", pass(len(blocking) == 0, fmt.Sprint(len(blocking))))
	if len(v.Reasons) > 0 {
		w("**Exit %d:** %s.\n\n", v.ExitCode, strings.Join(v.Reasons, "; "))
	}

	var changed, zeroChanged, unchanged []estimate.Resource
	for _, r := range res.Resources {
		switch {
		case r.Action == plan.NoOp:
			if r.Unpriced() == "" && !r.ZeroCost {
				unchanged = append(unchanged, r)
			}
		case r.ZeroCost && r.Unpriced() == "":
			zeroChanged = append(zeroChanged, r)
		default:
			changed = append(changed, r)
		}
	}

	if len(changed) > 0 {
		w("#### Changed resources\n\n| Resource | Action | Before | After | Delta | Breakdown |\n|---|---|---:|---:|---:|---|\n")
		for _, r := range changed {
			delta := money.Signed(r.Delta, 2)
			if r.Unpriced() != "" {
				delta = "?"
			}
			w("| `%s` | %s | %s | %s | %s | %s |\n", r.Address, r.Action, cell(&r.Before), cell(&r.After), delta, escape(breakdown(r)))
		}
		w("\n")
		billingDetails(w, changed)
	}

	var unpricedRows []estimate.Resource
	for _, r := range res.Resources {
		if r.Unpriced() != "" {
			unpricedRows = append(unpricedRows, r)
		}
	}
	if len(unpricedRows) > 0 {
		w("#### Unpriced resources\n\n| Resource | Action | Reason | Blocks the gate |\n|---|---|---|---|\n")
		for _, r := range unpricedRows {
			w("| `%s` | %s | %s | %s |\n", r.Address, r.Action, escape(r.Unpriced()), yesNo(r.Blocking))
		}
		w("\n")
	}

	if und := res.Undeclared(); len(und) > 0 {
		w("#### Usage premises not declared (counted as 0)\n\n")
		for _, u := range und {
			w("- `%s`: %s, set `%s` in the usage file\n", u.Address, u.Line, u.Premise)
		}
		w("\n")
	}

	if len(zeroChanged) > 0 {
		w("<details><summary>%s with no charge</summary>\n\n", plural(len(zeroChanged), "changed resource", "changed resources"))
		for _, r := range zeroChanged {
			w("- `%s` (%s)\n", r.Address, r.Action)
		}
		w("\n</details>\n\n")
	}
	if len(unchanged) > 0 {
		w("<details><summary>%s</summary>\n\n| Resource | Monthly |\n|---|---:|\n", plural(len(unchanged), "unchanged priced resource", "unchanged priced resources"))
		for _, r := range unchanged {
			w("| `%s` | %s |\n", r.Address, usd(r.After.Monthly))
		}
		w("\n</details>\n\n")
	}

	p := in.Pricing
	short := p.SHA256
	if len(short) > 12 {
		short = short[:12]
	}
	w("<sub>costgate %s · AWS Price List Bulk API snapshot `%s` as of %s (sha256 `%s`) · on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included.</sub>\n",
		in.Version, p.Region, p.AsOf, short)
	return b.String()
}

// billingDetails lists, in a collapsed block, the bill keys of the lines in
// the Breakdown column: SKU, usage type with the region prefix, billing rule
// and the description AWS prints on the invoice.
func billingDetails(w func(string, ...any), rows []estimate.Resource) {
	n := 0
	for _, r := range rows {
		n += len(shownSide(r).Lines)
	}
	if n == 0 {
		return
	}
	w("<details><summary>Billing keys of %s (SKU and invoice description)</summary>\n\n", plural(n, "cost line", "cost lines"))
	w("| Resource | Line | SKU | Usage type | Billing rule | Invoice description |\n|---|---|---|---|---|---|\n")
	for _, r := range rows {
		for _, l := range shownSide(r).Lines {
			w("| `%s` | %s | `%s` | `%s` | %s | %s |\n", r.Address, escape(l.Name), dash(l.SKU), dash(l.UsageType), dash(l.BillingRule), escape(dash(l.BillingDescription)))
		}
	}
	w("\n</details>\n\n")
}

// shownSide is the side the Breakdown column describes.
func shownSide(r estimate.Resource) *estimate.Side {
	if !r.After.Exists {
		return &r.Before
	}
	return &r.After
}

func dash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func statusText(s string) string {
	switch s {
	case StatusOverCeiling:
		return " · over ceiling"
	case StatusUnpriced:
		return " · unpriced resources"
	}
	return ""
}

func pass(ok bool, value string) string {
	if ok {
		return "✅ " + value
	}
	return "❌ " + value
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no (unchanged or deleted)"
}

// JSON report types.

type jsonLine struct {
	Name               string       `json:"name"`
	PriceKey           string       `json:"price_key"`
	SKU                string       `json:"sku"`
	UsageType          string       `json:"usage_type"`
	RateCode           string       `json:"rate_code"`
	BillingDescription string       `json:"billing_description"`
	BillingRule        string       `json:"billing_rule"`
	UnitPrice          json.Number  `json:"unit_price"`
	Quantity           json.Number  `json:"quantity"`
	Unit               string       `json:"unit"`
	Factor             *json.Number `json:"factor,omitempty"`
	Monthly            json.Number  `json:"monthly"`
	Premise            string       `json:"premise,omitempty"`
	Declared           bool         `json:"declared"`
	Note               string       `json:"note,omitempty"`
}

type jsonSide struct {
	Monthly  json.Number `json:"monthly"`
	Unpriced string      `json:"unpriced,omitempty"`
	Lines    []jsonLine  `json:"lines"`
}

type jsonResource struct {
	Address  string      `json:"address"`
	Type     string      `json:"type"`
	Action   plan.Action `json:"action"`
	ZeroCost bool        `json:"zero_cost"`
	Before   *jsonSide   `json:"before"`
	After    *jsonSide   `json:"after"`
	Delta    json.Number `json:"delta"`
	Blocking bool        `json:"blocking"`
}

type jsonReport struct {
	SchemaVersion int    `json:"schema_version"`
	Tool          string `json:"tool"`
	Version       string `json:"version"`
	Region        string `json:"region"`
	Currency      string `json:"currency"`
	Pricing       struct {
		AsOf    string           `json:"as_of"`
		SHA256  string           `json:"sha256"`
		Sources []catalog.Source `json:"sources"`
	} `json:"pricing"`
	Totals struct {
		Before json.Number `json:"monthly_before"`
		After  json.Number `json:"monthly_after"`
		Delta  json.Number `json:"monthly_delta"`
	} `json:"totals"`
	Ceilings struct {
		MaxDelta *json.Number `json:"max_monthly_delta"`
		MaxTotal *json.Number `json:"max_monthly_total"`
	} `json:"ceilings"`
	Status     string                     `json:"status"`
	ExitCode   int                        `json:"exit_code"`
	Reasons    []string                   `json:"reasons"`
	Incomplete bool                       `json:"incomplete"`
	Resources  []jsonResource             `json:"resources"`
	Undeclared []estimate.UndeclaredUsage `json:"usage_not_declared"`
}

func num(r *big.Rat) json.Number { return json.Number(money.Fixed(r, 4)) }

func numPtr(r *big.Rat) *json.Number {
	if r == nil {
		return nil
	}
	n := num(r)
	return &n
}

func exact(r *big.Rat) json.Number { return json.Number(money.Trim(r)) }

func toJSONLine(l estimate.Line) jsonLine {
	jl := jsonLine{Name: l.Name, PriceKey: l.PriceKey, SKU: l.SKU, UsageType: l.UsageType, RateCode: l.RateCode,
		BillingDescription: l.BillingDescription, BillingRule: l.BillingRule,
		UnitPrice: exact(l.UnitPrice), Quantity: exact(l.Quantity),
		Unit: l.Unit, Monthly: num(l.Monthly), Premise: l.Premise, Declared: l.Declared, Note: l.Note}
	if l.Factor != nil {
		f := exact(l.Factor)
		jl.Factor = &f
	}
	return jl
}

func toJSONSide(s estimate.Side) *jsonSide {
	if !s.Exists {
		return nil
	}
	out := &jsonSide{Monthly: num(s.Monthly), Unpriced: s.Unpriced, Lines: []jsonLine{}}
	for _, l := range s.Lines {
		out.Lines = append(out.Lines, toJSONLine(l))
	}
	return out
}

// JSON renders the machine-readable report with a trailing newline.
func JSON(in Input) ([]byte, error) {
	res := in.Result
	var r jsonReport
	r.SchemaVersion, r.Tool, r.Version = JSONSchemaVersion, "costgate", in.Version
	r.Region, r.Currency = res.Region, "USD"
	r.Pricing.AsOf, r.Pricing.SHA256, r.Pricing.Sources = in.Pricing.AsOf, in.Pricing.SHA256, in.Pricing.Sources
	r.Totals.Before, r.Totals.After, r.Totals.Delta = num(res.Before), num(res.After), num(res.Delta)
	r.Ceilings.MaxDelta, r.Ceilings.MaxTotal = numPtr(in.Ceilings.MaxDelta), numPtr(in.Ceilings.MaxTotal)
	r.Status, r.ExitCode, r.Reasons = in.Verdict.Status, in.Verdict.ExitCode, in.Verdict.Reasons
	if r.Reasons == nil {
		r.Reasons = []string{}
	}
	r.Incomplete = res.Incomplete()
	r.Resources = []jsonResource{}
	for _, x := range res.Resources {
		r.Resources = append(r.Resources, jsonResource{
			Address: x.Address, Type: x.Type, Action: x.Action, ZeroCost: x.ZeroCost,
			Before: toJSONSide(x.Before), After: toJSONSide(x.After), Delta: num(x.Delta), Blocking: x.Blocking,
		})
	}
	r.Undeclared = res.Undeclared()
	if r.Undeclared == nil {
		r.Undeclared = []estimate.UndeclaredUsage{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LineKind is the "kind" of every JSON Lines document.
const LineKind = "costgate.line"

// jsonLineDoc is one cost line with the report fields a standalone query
// needs. The line fields are the same as in the JSON report.
type jsonLineDoc struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Tool          string `json:"tool"`
	Version       string `json:"version"`
	Region        string `json:"region"`
	Currency      string `json:"currency"`
	PricingAsOf   string `json:"pricing_as_of"`
	PricingSHA256 string `json:"pricing_sha256"`
	Address       string `json:"address"`
	Type          string `json:"type"`
	Action        string `json:"action"`
	Side          string `json:"side"`
	jsonLine
}

// JSONLines renders one compact JSON document per cost line and per side
// (before, then after), in the order of the report. Unpriced and zero-cost
// resources have no cost line and appear only in the JSON report.
func JSONLines(in Input) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, r := range in.Result.Resources {
		for _, sd := range []struct {
			name string
			side estimate.Side
		}{{"before", r.Before}, {"after", r.After}} {
			if !sd.side.Exists {
				continue
			}
			for _, l := range sd.side.Lines {
				doc := jsonLineDoc{
					SchemaVersion: JSONSchemaVersion, Kind: LineKind, Tool: "costgate", Version: in.Version,
					Region: in.Result.Region, Currency: "USD", PricingAsOf: in.Pricing.AsOf, PricingSHA256: in.Pricing.SHA256,
					Address: r.Address, Type: r.Type, Action: string(r.Action), Side: sd.name, jsonLine: toJSONLine(l),
				}
				if err := enc.Encode(doc); err != nil {
					return nil, err
				}
			}
		}
	}
	return buf.Bytes(), nil
}
