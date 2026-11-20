package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
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
	Warnings []Warning
}

// Codes of the warnings RegionWarnings builds.
const (
	// WarningRegionMismatch: the plan deploys to a region costgate has no
	// price snapshot of.
	WarningRegionMismatch = "region_mismatch"
	// WarningRegionAssumed: resources whose region the plan does not state
	// were priced in a region the caller did not choose.
	WarningRegionAssumed = "region_assumed"
)

// Warning is a top-level notice: part of the estimate is probably not what
// the caller wants, and Fix says how to correct it.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// RegionWarnings builds one warning per region the plan deploys to that
// costgate has no price snapshot of, and one when resources whose region
// the plan does not state were priced in a region the caller did not choose.
func RegionWarnings(res *estimate.Result) []Warning {
	var out []Warning
	for _, m := range res.RegionMismatches {
		from := ""
		if m.Inferred {
			from = " (region taken from the region attribute or ARNs of its resources)"
		}
		msg := fmt.Sprintf("provider configuration %s deploys to %s%s, and costgate has no price snapshot of %s; %s in %s left unpriced (reason region)",
			strings.Join(m.Providers, ", "), m.Region, from, m.Region,
			plural(m.Resources, "resource", "resources"), m.Region)
		fix := fmt.Sprintf("run 'costgate pricing refresh --region %[1]s --out %[1]s.json.gz', then rerun with --pricing %[1]s.json.gz", m.Region)
		if !catalog.RegionSupported(m.Region) {
			fix = fmt.Sprintf("the public AWS Price List of this endpoint has no region %[1]s (China regions have their own); rerun with --pricing <snapshot of %[1]s>", m.Region)
		}
		out = append(out, Warning{Code: WarningRegionMismatch, Message: msg, Fix: fix})
	}
	if res.Assumed > 0 && res.RegionFrom != estimate.RegionFromFlag {
		why := "the only region the plan names"
		if res.RegionFrom == estimate.RegionFromDefault {
			switch len(res.PlanRegions) {
			case 0:
				why = "the default, because the plan names no region"
			case 1:
				why = "the default, because costgate has no price snapshot of " + res.PlanRegions[0] + ", the only region the plan names"
			default:
				why = "the default, because the plan names several regions (" + strings.Join(res.PlanRegions, ", ") + ")"
			}
		}
		out = append(out, Warning{Code: WarningRegionAssumed,
			Message: fmt.Sprintf("%s no region in the plan (no region attribute, no region in the provider configuration, no ARN yet); priced in %s, %s",
				plural(res.Assumed, "resource states", "resources state"), res.Region, why),
			Fix: "rerun with --region <region> if they deploy elsewhere"})
	}
	return out
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
	for _, wr := range in.Warnings {
		w("> ⚠️ **Warning (`%s`):** %s.<br>**Fix:** %s.\n\n", wr.Code, wr.Message, wr.Fix)
	}
	w("| Monthly cost (USD) | Before | After | Delta |\n|---|---:|---:|---:|\n")
	w("| Total | %s | %s | **%s** |\n\n", usd(res.Before), usd(res.After), money.Signed(res.Delta, 2))

	w("| Check | Limit | Result |\n|---|---:|---|\n")
	for _, c := range checks(in) {
		name := c.Name
		if c.Flag {
			name = "`" + name + "`"
		}
		w("| %s | %s | %s |\n", name, c.Limit, pass(c.OK, c.Value))
	}
	w("\n")
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

	multi := multiRegion(res)
	if len(changed) > 0 {
		w("#### Changed resources\n\n| Resource | Action | Before | After | Delta | Breakdown |\n|---|---|---:|---:|---:|---|\n")
		for _, r := range changed {
			delta := money.Signed(r.Delta, 2)
			if r.Unpriced() != "" {
				delta = "?"
			}
			w("| %s | %s | %s | %s | %s | %s |\n", resourceCell(r, multi), r.Action, cell(&r.Before), cell(&r.After), delta, escape(breakdown(r)))
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

	if len(res.IgnoredProviders) > 0 {
		w("<details><summary>%s of ignored providers (<code>--ignore-provider %s</code>), left out of the gate</summary>\n\n",
			plural(len(res.Ignored), "resource", "resources"), strings.Join(res.IgnoredProviders, ","))
		if len(res.Ignored) > 0 {
			w("| Resource | Action | Provider |\n|---|---|---|\n")
			for _, ig := range res.Ignored {
				w("| `%s` | %s | %s |\n", ig.Address, ig.Action, ig.Provider)
			}
			w("\n")
		}
		w("</details>\n\n")
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
			w("| %s | %s |\n", resourceCell(r, multi), usd(r.After.Monthly))
		}
		w("\n</details>\n\n")
	}

	ignored := ""
	if len(res.IgnoredProviders) > 0 {
		ignored = fmt.Sprintf(" · `--ignore-provider %s`: %s left out of the gate", strings.Join(res.IgnoredProviders, ","), plural(len(res.Ignored), "resource", "resources"))
	}
	w("<sub>costgate %s · %s%s · %s</sub>\n", in.Version, snapshotsText(in, "`"), ignored, disclaimer)
	return b.String()
}

// snapshotsText names the snapshots of the run for the footers: the one of
// the default region alone, or every one used. q quotes code spans.
func snapshotsText(in Input, q string) string {
	uses := in.Result.Snapshots
	if len(uses) <= 1 {
		p := in.Pricing
		if len(uses) == 1 {
			p = uses[0].Snapshot
		}
		return fmt.Sprintf("AWS Price List Bulk API snapshot %[1]s%[2]s%[1]s as of %[3]s (sha256 %[1]s%[4]s%[1]s)", q, p.Region, p.AsOf, shortSHA(p.SHA256))
	}
	parts := make([]string, 0, len(uses))
	for _, u := range uses {
		parts = append(parts, fmt.Sprintf("%[1]s%[2]s%[1]s as of %[3]s (sha256 %[1]s%[4]s%[1]s)", q, u.Region, u.Snapshot.AsOf, shortSHA(u.Snapshot.SHA256)))
	}
	return "AWS Price List Bulk API snapshots " + strings.Join(parts, ", ")
}

// multiRegion reports whether the resources of the report are priced in more
// than one region, so the tables name the region of each.
func multiRegion(res *estimate.Result) bool { return len(res.Snapshots) > 1 }

// resourceCell is the Resource column: the address, and the region when the
// report spans several.
func resourceCell(r estimate.Resource, multi bool) string {
	if multi {
		return fmt.Sprintf("`%s` · %s", r.Address, r.Region)
	}
	return "`" + r.Address + "`"
}

// disclaimer is the scope of the prices, in the footer of every report.
const disclaimer = "on-demand list prices, 730 h/month unless a premise overrides it · Savings Plans, Reserved Instances, credits, Free Tier and taxes are not included."

// check is one row of the checks table of the reports.
type check struct {
	Name  string `json:"name"`
	Flag  bool   `json:"flag"` // Name is a command line flag
	Limit string `json:"limit"`
	Value string `json:"value"`
	OK    bool   `json:"ok"`
}

func checks(in Input) []check {
	res, c := in.Result, in.Ceilings
	var out []check
	if c.MaxDelta != nil {
		out = append(out, check{Name: "--max-monthly-delta", Flag: true, Limit: usd(c.MaxDelta),
			Value: money.Signed(res.Delta, 2), OK: res.Delta.Cmp(c.MaxDelta) <= 0})
	}
	if c.MaxTotal != nil {
		out = append(out, check{Name: "--max-monthly-total", Flag: true, Limit: usd(c.MaxTotal),
			Value: usd(res.After), OK: res.After.Cmp(c.MaxTotal) <= 0 && !res.Incomplete()})
	}
	n := len(res.Blocking())
	return append(out, check{Name: "Unpriced resources created or changed", Limit: "0", Value: strconv.Itoa(n), OK: n == 0})
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
	Monthly        json.Number `json:"monthly"`
	Unpriced       string      `json:"unpriced,omitempty"`
	UnpricedReason string      `json:"unpriced_reason,omitempty"`
	Lines          []jsonLine  `json:"lines"`
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
	// Region is where the resource is priced; RegionAssumed is true when
	// the plan does not state it and the report region was used.
	Region        string `json:"region"`
	RegionAssumed bool   `json:"region_assumed,omitempty"`
}

// jsonSnapshot is a price snapshot the run used.
type jsonSnapshot struct {
	Region    string `json:"region"`
	AsOf      string `json:"as_of"`
	SHA256    string `json:"sha256"`
	Resources int    `json:"resources"`
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
	// Snapshots lists every snapshot used, the one of region included.
	Snapshots []jsonSnapshot `json:"snapshots"`
	Totals    struct {
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
	Warnings   []Warning                  `json:"warnings,omitempty"`
	Incomplete bool                       `json:"incomplete"`
	Resources  []jsonResource             `json:"resources"`
	Undeclared []estimate.UndeclaredUsage `json:"usage_not_declared"`
	// Present only with --ignore-provider.
	IgnoredProviders []string            `json:"ignored_providers,omitempty"`
	Ignored          *[]estimate.Ignored `json:"ignored,omitempty"`
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
	out := &jsonSide{Monthly: num(s.Monthly), Unpriced: s.Unpriced, UnpricedReason: s.Reason, Lines: []jsonLine{}}
	for _, l := range s.Lines {
		out.Lines = append(out.Lines, toJSONLine(l))
	}
	return out
}

// JSON renders the machine-readable report with a trailing newline.
func JSON(in Input) ([]byte, error) {
	r := buildReport(in)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildReport is the JSON report; the HTML page embeds the same value.
func buildReport(in Input) jsonReport {
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
			Region: x.Region, RegionAssumed: x.RegionAssumed,
		})
	}
	r.Snapshots = []jsonSnapshot{}
	for _, u := range res.Snapshots {
		r.Snapshots = append(r.Snapshots, jsonSnapshot{Region: u.Region, AsOf: u.Snapshot.AsOf, SHA256: u.Snapshot.SHA256, Resources: u.Resources})
	}
	r.Undeclared = res.Undeclared()
	if r.Undeclared == nil {
		r.Undeclared = []estimate.UndeclaredUsage{}
	}
	r.Warnings = in.Warnings
	if len(res.IgnoredProviders) > 0 {
		r.IgnoredProviders = res.IgnoredProviders
		ignored := append([]estimate.Ignored{}, res.Ignored...)
		r.Ignored = &ignored
	}
	return r
}

// Kinds of JSON Lines documents. Cost lines are LineKind; the other kinds
// appear only when there is a warning or --ignore-provider is set, so a
// query over cost lines filters on kind.
const (
	LineKind    = "costgate.line"
	WarningKind = "costgate.warning"
	IgnoredKind = "costgate.ignored"
)

// jsonDocHeader is the part every JSON Lines document shares.
type jsonDocHeader struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Tool          string `json:"tool"`
	Version       string `json:"version"`
	Region        string `json:"region"`
	Currency      string `json:"currency"`
	PricingAsOf   string `json:"pricing_as_of"`
	PricingSHA256 string `json:"pricing_sha256"`
}

type jsonWarningDoc struct {
	jsonDocHeader
	Warning
}

type jsonIgnoredDoc struct {
	jsonDocHeader
	estimate.Ignored
}

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
// resources have no cost line and appear only in the JSON report. Warnings
// come first (kind costgate.warning) and resources of ignored providers
// last (kind costgate.ignored).
func JSONLines(in Input) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	header := func(kind string) jsonDocHeader {
		return jsonDocHeader{SchemaVersion: JSONSchemaVersion, Kind: kind, Tool: "costgate", Version: in.Version,
			Region: in.Result.Region, Currency: "USD", PricingAsOf: in.Pricing.AsOf, PricingSHA256: in.Pricing.SHA256}
	}
	for _, wr := range in.Warnings {
		if err := enc.Encode(jsonWarningDoc{header(WarningKind), wr}); err != nil {
			return nil, err
		}
	}
	for _, r := range in.Result.Resources {
		for _, doc := range lineDocs(in, r) {
			s, err := encodeLineDoc(doc)
			if err != nil {
				return nil, err
			}
			buf.WriteString(s)
			buf.WriteByte('\n')
		}
	}
	for _, ig := range in.Result.Ignored {
		if err := enc.Encode(jsonIgnoredDoc{header(IgnoredKind), ig}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// lineDocs returns the JSON Lines documents of one resource: its cost lines,
// before side first.
func lineDocs(in Input, r estimate.Resource) []jsonLineDoc {
	var out []jsonLineDoc
	for _, sd := range []struct {
		name string
		side estimate.Side
	}{{"before", r.Before}, {"after", r.After}} {
		if !sd.side.Exists {
			continue
		}
		// A line was priced with the snapshot of the region of its side.
		snap := in.Result.Snapshot(sd.side.Region)
		if snap == nil {
			snap = in.Pricing
		}
		for _, l := range sd.side.Lines {
			out = append(out, jsonLineDoc{
				SchemaVersion: JSONSchemaVersion, Kind: LineKind, Tool: "costgate", Version: in.Version,
				Region: snap.Region, Currency: "USD", PricingAsOf: snap.AsOf, PricingSHA256: snap.SHA256,
				Address: r.Address, Type: r.Type, Action: string(r.Action), Side: sd.name, jsonLine: toJSONLine(l),
			})
		}
	}
	return out
}

// encodeLineDoc is one JSON Lines document without the trailing newline.
func encodeLineDoc(doc jsonLineDoc) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
