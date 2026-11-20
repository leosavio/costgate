package report

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"html"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/leosavio/costgate/internal/estimate"
	"github.com/leosavio/costgate/internal/money"
	"github.com/leosavio/costgate/internal/plan"
)

// The page template, its style and its script, inlined into every report.
var (
	//go:embed assets/page.html
	pageHTML string
	//go:embed assets/page.css
	pageCSS string
	//go:embed assets/page.js
	pageJS string
)

// RootModule is the module name of resources outside any module.
const RootModule = "(root)"

// htmlData is the JSON embedded in the page: the JSON report of the run and
// the values the page shows that need exact decimals (amounts already
// formatted, group totals, bar lengths) or byte equality with another output
// (the JSON Lines documents of the export).
type htmlData struct {
	Report jsonReport `json:"report"`
	View   htmlView   `json:"view"`
}

type htmlView struct {
	Totals htmlTotals `json:"totals"`
	Checks []check    `json:"checks"`
	// Resources is parallel to report.resources.
	Resources []htmlRow        `json:"resources"`
	Modules   []htmlGroup      `json:"modules"`
	Types     []htmlGroup      `json:"types"`
	Unpriced  []htmlUnpriced   `json:"unpriced"`
	Premises  []htmlPremise    `json:"premises"`
	Region    htmlRegion       `json:"region"`
	CSVHeader []string         `json:"csv_header"`
	Footer    string           `json:"footer"`
	Short     string           `json:"sha256_short"`
	Status    htmlStatusLabels `json:"status_text"`
	// SnapshotsText names the snapshots of the run, as the Markdown footer.
	SnapshotsText string `json:"snapshots_text"`
}

type htmlStatusLabels struct {
	Title    string `json:"title"`
	Headline string `json:"headline"`
}

type htmlTotals struct {
	Before string `json:"before"`
	After  string `json:"after"`
	Delta  string `json:"delta"`
}

type htmlRow struct {
	Module string `json:"module"`
	Before string `json:"before"` // amount, "?" when unpriced, "–" when the side does not exist
	After  string `json:"after"`
	Delta  string `json:"delta"` // signed amount, "?" when unpriced
	// Kind is "priced", "unpriced" or "no_charge".
	Kind  string     `json:"kind"`
	Sides []htmlSide `json:"sides"`
	// Export holds the JSON Lines documents of the resource, before side
	// first, as `--json-lines` writes them, and the same values as CSV cells.
	Export []htmlExport `json:"export"`
}

type htmlSide struct {
	Side    string `json:"side"` // "before" or "after", key in report.resources[]
	Monthly string `json:"monthly"`
	// Lines is parallel to report.resources[].<side>.lines.
	Lines []htmlLine `json:"lines"`
}

type htmlLine struct {
	Formula string `json:"formula"`
	Monthly string `json:"monthly"`
}

type htmlExport struct {
	JSONL string   `json:"jsonl"`
	CSV   []string `json:"csv"`
}

type htmlGroup struct {
	Key      string `json:"key"`
	Count    int    `json:"count"`
	Changed  int    `json:"changed"`
	Unpriced int    `json:"unpriced"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Delta    string `json:"delta"`
	Sign     int    `json:"sign"`
	// Bar lengths in per mille of the largest group: monthly after, and
	// absolute delta.
	BarAfter int `json:"bar_after"`
	BarDelta int `json:"bar_delta"`
}

type htmlUnpriced struct {
	Category  string `json:"category"`
	Title     string `json:"title"`
	Hint      string `json:"hint"`
	Blocking  int    `json:"blocking"`
	Resources []int  `json:"resources"`
}

// htmlPremise points at a cost line whose quantity or factor is a usage
// premise, on the side the report describes (after, or before for a delete).
type htmlPremise struct {
	Resource int    `json:"resource"`
	Side     string `json:"side"`
	Line     int    `json:"line"`
	// PerUnit is what one unit of a missing premise would add per month.
	PerUnit string `json:"per_unit,omitempty"`
}

type htmlRegion struct {
	// Priced is the region of the resources the plan does not place, and
	// From how it was chosen (estimate.RegionFrom*).
	Priced   string `json:"priced"`
	From     string `json:"from"`
	Snapshot string `json:"snapshot"`
	// Snapshots lists every snapshot used; Multi is true when resources
	// are priced in more than one region.
	Snapshots []htmlSnapshot `json:"snapshots"`
	Multi     bool           `json:"multi"`
	// Mismatch lists the resources unpriced because costgate has no
	// snapshot of their region.
	Mismatch []int `json:"mismatch"`
}

type htmlSnapshot struct {
	Region    string `json:"region"`
	AsOf      string `json:"as_of"`
	SHA256    string `json:"sha256"`
	Short     string `json:"sha256_short"`
	Resources int    `json:"resources"`
}

// HTML renders the report as one self-contained page: the data, the style
// and the script are inline, and the Content-Security-Policy allows no
// network request. The bytes depend only on the input.
func HTML(in Input) ([]byte, error) {
	view, err := buildView(in)
	if err != nil {
		return nil, err
	}
	// json.Marshal escapes <, > and & (and U+2028, U+2029), so no string of
	// the plan can close the <script> element that carries the data.
	data, err := json.Marshal(htmlData{Report: buildReport(in), View: view})
	if err != nil {
		return nil, err
	}
	csp := "default-src 'none'; script-src " + cspHash(pageJS) + "; style-src " + cspHash(pageCSS) +
		"; base-uri 'none'; form-action 'none'"
	page := strings.NewReplacer(
		"__CSP__", csp,
		"__VERSION__", html.EscapeString(in.Version),
		"__CSS__", pageCSS,
		"__JS__", pageJS,
		"__DATA__", string(data),
	).Replace(pageHTML)
	return []byte(page), nil
}

// cspHash is the CSP source that allows exactly one inline element.
func cspHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

func buildView(in Input) (htmlView, error) {
	res, p := in.Result, in.Pricing
	v := htmlView{
		Totals:    htmlTotals{Before: usd(res.Before), After: usd(res.After), Delta: money.Signed(res.Delta, 2)},
		Checks:    checks(in),
		Resources: []htmlRow{},
		Unpriced:  []htmlUnpriced{},
		Premises:  []htmlPremise{},
		Region: htmlRegion{Priced: res.Region, From: res.RegionFrom, Snapshot: p.Region, Snapshots: []htmlSnapshot{},
			Multi: multiRegion(res), Mismatch: []int{}},
		CSVHeader:     csvHeader,
		Footer:        disclaimer,
		SnapshotsText: snapshotsText(in, ""),
		Short:         shortSHA(p.SHA256),
		Status: htmlStatusLabels{
			Title:    statusTitle(in.Verdict.Status),
			Headline: signedUSD(res.Delta) + "/month",
		},
	}
	for _, u := range res.Snapshots {
		v.Region.Snapshots = append(v.Region.Snapshots, htmlSnapshot{Region: u.Region, AsOf: u.Snapshot.AsOf,
			SHA256: u.Snapshot.SHA256, Short: shortSHA(u.Snapshot.SHA256), Resources: u.Resources})
	}
	byCategory := map[string]int{}
	for _, c := range unpricedCategories {
		byCategory[c.key] = len(v.Unpriced)
		v.Unpriced = append(v.Unpriced, htmlUnpriced{Category: c.key, Title: c.title, Hint: c.hint, Resources: []int{}})
	}
	for i, r := range res.Resources {
		row := htmlRow{Module: moduleOf(r.Address), Before: cell(&r.Before), After: cell(&r.After),
			Delta: money.Signed(r.Delta, 2), Kind: "priced", Sides: []htmlSide{}, Export: []htmlExport{}}
		// The side the Markdown Breakdown column describes: after, or before for a delete.
		shown := "after"
		if !r.After.Exists {
			shown = "before"
		}
		switch {
		case r.Unpriced() != "":
			row.Kind, row.Delta = "unpriced", "?"
		case r.ZeroCost && len(shownSide(r).Lines) == 0:
			row.Kind = "no_charge"
		}
		for _, sd := range []struct {
			name string
			side *estimate.Side
		}{{"before", &r.Before}, {"after", &r.After}} {
			if !sd.side.Exists {
				continue
			}
			hs := htmlSide{Side: sd.name, Monthly: cell(sd.side), Lines: []htmlLine{}}
			for j, l := range sd.side.Lines {
				hs.Lines = append(hs.Lines, htmlLine{Formula: formula(l), Monthly: usd(l.Monthly)})
				if l.Premise != "" && sd.name == shown {
					pr := htmlPremise{Resource: i, Side: sd.name, Line: j}
					if !l.Declared {
						pr.PerUnit = perUnit(l)
					}
					v.Premises = append(v.Premises, pr)
				}
			}
			row.Sides = append(row.Sides, hs)
		}
		for _, doc := range lineDocs(in, r) {
			s, err := encodeLineDoc(doc)
			if err != nil {
				return htmlView{}, err
			}
			row.Export = append(row.Export, htmlExport{JSONL: s, CSV: csvRecord(doc)})
		}
		if r.Unpriced() != "" {
			cat := unpricedCategory(r.UnpricedReason())
			g := &v.Unpriced[byCategory[cat]]
			g.Resources = append(g.Resources, i)
			if r.Blocking {
				g.Blocking++
			}
			if cat == estimate.ReasonRegion {
				v.Region.Mismatch = append(v.Region.Mismatch, i)
			}
		}
		v.Resources = append(v.Resources, row)
	}
	// Drop the empty categories, keeping the fixed order of the others.
	kept := v.Unpriced[:0]
	for _, g := range v.Unpriced {
		if len(g.Resources) > 0 {
			kept = append(kept, g)
		}
	}
	v.Unpriced = kept
	v.Modules = groups(res.Resources, func(r estimate.Resource) string { return moduleOf(r.Address) })
	v.Types = groups(res.Resources, func(r estimate.Resource) string { return r.Type })
	return v, nil
}

func statusTitle(s string) string {
	switch s {
	case StatusOverCeiling:
		return "over ceiling"
	case StatusUnpriced:
		return "unpriced resources"
	}
	return "ok"
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// formula is the computation of a cost line, as in the Breakdown column of
// the Markdown report without the line name.
func formula(l estimate.Line) string {
	if l.Premise != "" && !l.Declared {
		return "usage not declared (" + l.Premise + "), counted as 0"
	}
	parts := []string{money.Trim(l.Quantity) + " " + l.Unit, "$" + money.Trim(l.UnitPrice)}
	if l.Factor != nil {
		parts = append(parts, money.Trim(l.Factor))
	}
	s := strings.Join(parts, " × ") + " = " + usd(l.Monthly)
	if l.Note != "" {
		s += " (" + l.Note + ")"
	}
	return s
}

// perUnit is the monthly cost of one unit of the line quantity.
func perUnit(l estimate.Line) string {
	f := []*big.Rat{l.UnitPrice}
	if l.Factor != nil {
		f = append(f, l.Factor)
	}
	return "US$ " + money.Trim(money.Mul(f...)) + " per " + l.Unit
}

// Unpriced reasons grouped by what the reader can do about them, keyed by
// the estimate.Reason* code (unpriced_reason in the JSON report). An unknown
// code falls in the last category.
var unpricedCategories = []struct{ key, title, hint string }{
	{estimate.ReasonProvider, "Provider is not the AWS provider",
		"Resources of other providers (helm, kubectl, kubernetes…) have no AWS list price. --ignore-provider <name> leaves them out of the estimate and of the gate."},
	{estimate.ReasonType, "Resource type not in the pricing map",
		"costgate has no formula for the type, so it cannot prove a cost."},
	{estimate.ReasonRegion, "No price snapshot of the resource region",
		"costgate embeds every region of the public Price List; pass --pricing with a snapshot of this one (see the warning)."},
	{estimate.ReasonPrice, "Price missing in the snapshot",
		"The formula needs a price key the snapshot does not have."},
	{estimate.ReasonConfig, "Configuration not modeled or unknown at plan time",
		"An attribute the formula needs is unknown until apply, or this variant of the type is not modeled."},
}

func unpricedCategory(code string) string {
	for _, c := range unpricedCategories {
		if c.key == code {
			return c.key
		}
	}
	return unpricedCategories[len(unpricedCategories)-1].key
}

// moduleOf returns the module path of a resource address, such as
// `module.net` or `module.app["a.b"].module.db`, or RootModule.
func moduleOf(addr string) string {
	end, i := 0, 0
	for strings.HasPrefix(addr[i:], "module.") {
		j := i + len("module.")
		for j < len(addr) && addr[j] != '.' && addr[j] != '[' {
			j++
		}
		for j < len(addr) && addr[j] == '[' {
			k := closingBracket(addr, j)
			if k < 0 {
				return RootModule
			}
			j = k + 1
		}
		end = j
		if j >= len(addr) || addr[j] != '.' {
			break
		}
		i = j + 1
	}
	if end == 0 {
		return RootModule
	}
	return addr[:end]
}

// closingBracket returns the index of the ']' that closes the '[' at open,
// skipping a quoted key with escapes, or -1.
func closingBracket(s string, open int) int {
	j := open + 1
	if j < len(s) && s[j] == '"' {
		for j++; j < len(s) && s[j] != '"'; j++ {
			if s[j] == '\\' {
				j++
			}
		}
		j++
	}
	for ; j < len(s); j++ {
		if s[j] == ']' {
			return j
		}
	}
	return -1
}

// groups sums the resources by key, largest monthly total after the change
// first, then by key.
func groups(rs []estimate.Resource, key func(estimate.Resource) string) []htmlGroup {
	type acc struct {
		key                   string
		before, after, delta  *big.Rat
		count, changed, unpri int
	}
	var list []*acc
	index := map[string]*acc{}
	for _, r := range rs {
		k := key(r)
		a := index[k]
		if a == nil {
			a = &acc{key: k, before: money.Zero(), after: money.Zero(), delta: money.Zero()}
			index[k] = a
			list = append(list, a)
		}
		a.count++
		if r.Action != plan.NoOp {
			a.changed++
		}
		if r.Unpriced() != "" {
			a.unpri++
		}
		a.before = money.Add(a.before, r.Before.Monthly)
		a.after = money.Add(a.after, r.After.Monthly)
		a.delta = money.Add(a.delta, r.Delta)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if c := list[i].after.Cmp(list[j].after); c != 0 {
			return c > 0
		}
		return list[i].key < list[j].key
	})
	maxAfter, maxDelta := money.Zero(), money.Zero()
	for _, a := range list {
		if a.after.Cmp(maxAfter) > 0 {
			maxAfter = a.after
		}
		if d := new(big.Rat).Abs(a.delta); d.Cmp(maxDelta) > 0 {
			maxDelta = d
		}
	}
	out := []htmlGroup{}
	for _, a := range list {
		out = append(out, htmlGroup{
			Key: a.key, Count: a.count, Changed: a.changed, Unpriced: a.unpri,
			Before: usd(a.before), After: usd(a.after), Delta: money.Signed(a.delta, 2), Sign: a.delta.Sign(),
			BarAfter: perMille(a.after, maxAfter), BarDelta: perMille(new(big.Rat).Abs(a.delta), maxDelta),
		})
	}
	return out
}

// perMille is x/max in thousandths, rounded down, and at least 1 when x is
// positive so a small amount stays visible.
func perMille(x, max *big.Rat) int {
	if max.Sign() == 0 || x.Sign() <= 0 {
		return 0
	}
	q := new(big.Rat).Quo(money.Mul(x, money.Int(1000)), max)
	n := int(new(big.Int).Quo(q.Num(), q.Denom()).Int64())
	if n == 0 {
		return 1
	}
	return n
}

// csvHeader is the CSV export header: the fields of a JSON Lines document,
// in the same order.
var csvHeader = []string{"schema_version", "kind", "tool", "version", "region", "currency",
	"pricing_as_of", "pricing_sha256", "address", "type", "action", "side", "name", "price_key",
	"sku", "usage_type", "rate_code", "billing_description", "billing_rule", "unit_price",
	"quantity", "unit", "factor", "monthly", "premise", "declared", "note"}

func csvRecord(d jsonLineDoc) []string {
	factor := ""
	if d.Factor != nil {
		factor = d.Factor.String()
	}
	return []string{strconv.Itoa(d.SchemaVersion), d.Kind, d.Tool, d.Version, d.Region, d.Currency,
		d.PricingAsOf, d.PricingSHA256, d.Address, d.Type, d.Action, d.Side, d.Name, d.PriceKey,
		d.SKU, d.UsageType, d.RateCode, d.BillingDescription, d.BillingRule, d.UnitPrice.String(),
		d.Quantity.String(), d.Unit, factor, d.Monthly.String(), d.Premise, strconv.FormatBool(d.Declared), d.Note}
}
