package catalog

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/leosavio/costgate/internal/money"
)

// DefaultBaseURL is the public, unauthenticated Price List Bulk API host.
const DefaultBaseURL = "https://pricing.us-east-1.amazonaws.com"

// Fetcher downloads offer files over HTTP. No AWS credential is involved.
type Fetcher struct {
	BaseURL   string
	Client    *http.Client
	UserAgent string
}

func (f Fetcher) get(ctx context.Context, path string) (*http.Response, string, error) {
	url := strings.TrimRight(f.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, url, err
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, url, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, url, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return resp, url, nil
}

func (f Fetcher) getJSON(ctx context.Context, path string, v any) error {
	resp, url, err := f.get(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	return nil
}

type offerIndex struct {
	Offers map[string]struct {
		CurrentVersionURL     string `json:"currentVersionUrl"`
		CurrentRegionIndexURL string `json:"currentRegionIndexUrl"`
	} `json:"offers"`
}

type regionIndex struct {
	Regions map[string]struct {
		CurrentVersionURL string `json:"currentVersionUrl"`
	} `json:"regions"`
}

type fileKey struct{ service, scope string }

// Refresh downloads the offer files the rules need and extracts only the
// matching on-demand prices. The CSV form is streamed, so the 300 MB EC2 file
// never sits in memory.
func Refresh(ctx context.Context, f Fetcher, region string, rules []Rule) (*Snapshot, error) {
	prefixes, ok := regionPrefixes[region]
	if !ok {
		return nil, fmt.Errorf("region %q: usage type prefixes unknown (supported: %s)", region, strings.Join(supportedRegions(), ", "))
	}
	var idx offerIndex
	if err := f.getJSON(ctx, "/offers/v1.0/aws/index.json", &idx); err != nil {
		return nil, err
	}

	groups := map[fileKey][]Rule{}
	var order []fileKey
	for _, r := range rules {
		k := fileKey{r.Service, r.Scope}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].service != order[j].service {
			return order[i].service < order[j].service
		}
		return order[i].scope < order[j].scope
	})

	snap := &Snapshot{SchemaVersion: SchemaVersion, Region: region, Currency: "USD", Prices: map[string]Price{}}
	for _, k := range order {
		offer, ok := idx.Offers[k.service]
		if !ok {
			return nil, fmt.Errorf("offer %s not in the Price List index", k.service)
		}
		var jsonPath string
		switch k.scope {
		case ScopeGlobal:
			jsonPath = offer.CurrentVersionURL
		case ScopeRegion:
			var ri regionIndex
			if err := f.getJSON(ctx, offer.CurrentRegionIndexURL, &ri); err != nil {
				return nil, err
			}
			reg, ok := ri.Regions[region]
			if !ok {
				return nil, fmt.Errorf("offer %s has no file for region %s", k.service, region)
			}
			jsonPath = reg.CurrentVersionURL
		default:
			return nil, fmt.Errorf("rule scope %q invalid", k.scope)
		}
		csvPath := strings.TrimSuffix(jsonPath, ".json") + ".csv"
		resp, url, err := f.get(ctx, csvPath)
		if err != nil {
			return nil, err
		}
		src, prices, err := extract(resp.Body, k, region, prefixes, groups[k])
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", url, err)
		}
		src.Path = csvPath
		snap.Sources = append(snap.Sources, src)
		for key, p := range prices {
			snap.Prices[key] = p
		}
	}
	snap.Seal()
	return snap, nil
}

func supportedRegions() []string {
	out := make([]string, 0, len(regionPrefixes))
	for r := range regionPrefixes {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

var requiredColumns = []string{"SKU", "TermType", "PriceDescription", "StartingRange", "EndingRange", "Unit", "PricePerUnit", "Currency", "usageType", "operation", "RateCode", "serviceName", "Location"}

// extract reads one CSV offer file: five metadata lines, a header, then one
// row per price dimension.
func extract(r io.Reader, k fileKey, region string, prefixes []string, rules []Rule) (Source, map[string]Price, error) {
	cr := csv.NewReader(bufio.NewReaderSize(r, 1<<20))
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true

	src := Source{Service: k.service, Scope: k.scope}
	meta := map[string]string{}
	for i := 0; i < 5; i++ {
		rec, err := cr.Read()
		if err != nil {
			return src, nil, fmt.Errorf("metadata line %d: %w", i+1, err)
		}
		if len(rec) >= 2 {
			meta[rec[0]] = rec[1]
		}
	}
	if meta["FormatVersion"] != "v1.0" {
		return src, nil, fmt.Errorf("unexpected FormatVersion %q", meta["FormatVersion"])
	}
	if meta["OfferCode"] != k.service {
		return src, nil, fmt.Errorf("OfferCode %q, want %q", meta["OfferCode"], k.service)
	}
	src.Version = meta["Version"]
	src.PublicationDate = meta["Publication Date"]

	header, err := cr.Read()
	if err != nil {
		return src, nil, fmt.Errorf("header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	need := append([]string{}, requiredColumns...)
	for _, rule := range rules {
		for c := range rule.Match {
			need = append(need, c)
		}
	}
	for _, c := range need {
		if _, ok := col[c]; !ok {
			return src, nil, fmt.Errorf("column %q missing", c)
		}
	}
	regionCol, hasRegionCol := col["Region Code"]

	field := func(rec []string, name string) string {
		i := col[name]
		if i < len(rec) {
			return rec[i]
		}
		return ""
	}

	prices := map[string]Price{}
	tiers := map[string]map[string]Tier{} // key -> start -> tier
	tierSKU := map[string]string{}
	hits := make([]int, len(rules))
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return src, nil, err
		}
		if field(rec, "TermType") != "OnDemand" {
			continue
		}
		if k.scope == ScopeRegion && hasRegionCol && regionCol < len(rec) && rec[regionCol] != region {
			continue
		}
		rawUT := field(rec, "usageType")
		ut := normalizeUsageType(rawUT, prefixes)
		op := field(rec, "operation")
		start := field(rec, "StartingRange")
		for i, rule := range rules {
			if rule.UsagePrefix {
				if !strings.HasPrefix(ut, rule.UsageType) {
					continue
				}
			} else if ut != rule.UsageType {
				continue
			}
			if op != rule.Operation {
				continue
			}
			matched := true
			for c, v := range rule.Match {
				if field(rec, c) != v {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			if cur := field(rec, "Currency"); cur != "USD" {
				return src, nil, fmt.Errorf("rule %s: currency %q", rule, cur)
			}
			usd := field(rec, "PricePerUnit")
			if _, err := money.Parse(usd); err != nil {
				return src, nil, fmt.Errorf("rule %s: sku %s: %w", rule, field(rec, "SKU"), err)
			}
			end := field(rec, "EndingRange")
			if end == "Inf" {
				end = ""
			}
			key := Key(k.service, ut, op)
			if start != "0" {
				// A later tier: kept for the bill, not used by the estimate.
				if _, err := money.Parse(start); err != nil {
					return src, nil, fmt.Errorf("rule %s: sku %s: StartingRange %q: %w", rule, field(rec, "SKU"), start, err)
				}
				t := Tier{Start: start, End: end, USD: usd, RateCode: field(rec, "RateCode"), Description: field(rec, "PriceDescription")}
				if tiers[key] == nil {
					tiers[key] = map[string]Tier{}
				}
				if prev, dup := tiers[key][start]; dup && prev != t {
					return src, nil, fmt.Errorf("rule %s: key %s tier %s is ambiguous (rate code %s and %s); tighten the rule", rule, key, start, prev.RateCode, t.RateCode)
				}
				tiers[key][start] = t
				if prev, ok := tierSKU[key]; ok && prev != field(rec, "SKU") {
					return src, nil, fmt.Errorf("rule %s: key %s has tiers from sku %s and %s; tighten the rule", rule, key, prev, field(rec, "SKU"))
				}
				tierSKU[key] = field(rec, "SKU")
				continue
			}
			p := Price{
				SKU:         field(rec, "SKU"),
				Unit:        field(rec, "Unit"),
				USD:         usd,
				TierEnd:     end,
				Description: field(rec, "PriceDescription"),
				RateCode:    field(rec, "RateCode"),
				UsageType:   rawUT,
				ServiceName: field(rec, "serviceName"),
				Location:    field(rec, "Location"),
			}
			if prev, dup := prices[key]; dup && !sameFirstTier(prev, p) {
				return src, nil, fmt.Errorf("rule %s: key %s is ambiguous (sku %s and %s); tighten the rule", rule, key, prev.SKU, p.SKU)
			}
			prices[key] = p
			hits[i]++
		}
	}
	for i, rule := range rules {
		if hits[i] == 0 {
			return src, nil, fmt.Errorf("rule %s matched no on-demand price; the offer changed, update internal/catalog/rules.go", rule)
		}
	}
	for key, byStart := range tiers {
		p, ok := prices[key]
		if !ok {
			return src, nil, fmt.Errorf("key %s has a tier starting above 0 but no first tier", key)
		}
		if tierSKU[key] != p.SKU {
			return src, nil, fmt.Errorf("key %s: tiers of sku %s do not belong to the first tier sku %s", key, tierSKU[key], p.SKU)
		}
		p.Tiers = make([]Tier, 0, len(byStart))
		for _, t := range byStart {
			p.Tiers = append(p.Tiers, t)
		}
		sort.Slice(p.Tiers, func(i, j int) bool {
			a, _ := money.Parse(p.Tiers[i].Start)
			b, _ := money.Parse(p.Tiers[j].Start)
			return a.Cmp(b) < 0
		})
		prices[key] = p
	}
	return src, prices, nil
}
