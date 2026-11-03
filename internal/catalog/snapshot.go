// Package catalog holds the versioned price snapshot and the refresh logic
// that builds it from the public AWS Price List Bulk API.
package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/leosavio/costgate/internal/money"
)

// SchemaVersion is the snapshot format this build writes. Version 2 adds the
// billing fields (rate code, raw usage type, service name, location, extra
// tiers). Every v2 field is omitempty, so a v1 file decodes into the same
// struct and keeps its digest: Load accepts MinSchemaVersion..SchemaVersion.
const SchemaVersion = 2

// MinSchemaVersion is the oldest snapshot format this build still reads.
const MinSchemaVersion = 1

// Source identifies one offer file the snapshot was extracted from.
type Source struct {
	Service         string `json:"service"`
	Scope           string `json:"scope"`
	Version         string `json:"version"`
	PublicationDate string `json:"publication_date"`
	// Path is relative to the Bulk API host, so the snapshot does not
	// depend on the mirror it was fetched from.
	Path string `json:"path"`
}

// Price is one on-demand price dimension. USD, TierEnd, RateCode and
// Description describe the first tier, the one the estimator uses; Tiers
// lists the following tiers, sorted by start.
type Price struct {
	SKU         string `json:"sku"`
	Unit        string `json:"unit"`
	USD         string `json:"usd"`
	TierEnd     string `json:"tier_end,omitempty"`
	Description string `json:"description"`
	// v2 fields, all from the offer file row.
	RateCode    string `json:"rate_code,omitempty"`
	UsageType   string `json:"usage_type,omitempty"` // with the region prefix, as in the bill
	ServiceName string `json:"service_name,omitempty"`
	Location    string `json:"location,omitempty"`
	Tiers       []Tier `json:"tiers,omitempty"`
}

// Tier is a price tier after the first one.
type Tier struct {
	Start       string `json:"start"`
	End         string `json:"end,omitempty"` // empty: no upper bound
	USD         string `json:"usd"`
	RateCode    string `json:"rate_code"`
	Description string `json:"description"`
}

// sameFirstTier compares the scalar fields of two prices.
func sameFirstTier(a, b Price) bool {
	return a.SKU == b.SKU && a.Unit == b.Unit && a.USD == b.USD && a.TierEnd == b.TierEnd &&
		a.Description == b.Description && a.RateCode == b.RateCode && a.UsageType == b.UsageType &&
		a.ServiceName == b.ServiceName && a.Location == b.Location
}

// Snapshot is the content of pricing/<region>.json.
type Snapshot struct {
	SchemaVersion int              `json:"schema_version"`
	Region        string           `json:"region"`
	Currency      string           `json:"currency"`
	AsOf          string           `json:"as_of"`
	SHA256        string           `json:"sha256"`
	Sources       []Source         `json:"sources"`
	Prices        map[string]Price `json:"prices"`

	parsed map[string]*big.Rat
}

// Key builds the lookup key of a price: offer code, usage type without the
// region prefix, and operation.
func Key(service, usageType, operation string) string {
	return service + "|" + usageType + "|" + operation
}

// Digest is the sha256 of the canonical encoding of region, currency,
// sources and prices. encoding/json sorts map keys, so it is deterministic.
func (s *Snapshot) Digest() string {
	b, err := json.Marshal(struct {
		Region   string           `json:"region"`
		Currency string           `json:"currency"`
		Sources  []Source         `json:"sources"`
		Prices   map[string]Price `json:"prices"`
	}{s.Region, s.Currency, s.Sources, s.Prices})
	if err != nil {
		panic(err) // only plain strings: cannot fail
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Seal sorts sources, derives as_of and stores the digest.
func (s *Snapshot) Seal() {
	sort.Slice(s.Sources, func(i, j int) bool {
		a, b := s.Sources[i], s.Sources[j]
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		return a.Scope < b.Scope
	})
	s.AsOf = ""
	for _, src := range s.Sources {
		if src.PublicationDate > s.AsOf {
			s.AsOf = src.PublicationDate
		}
	}
	s.SHA256 = s.Digest()
}

// Encode returns the indented JSON form with a trailing newline.
func (s *Snapshot) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Load decodes and validates a snapshot: schema version, digest and every
// price value.
func Load(data []byte) (*Snapshot, error) {
	// Version first, without DisallowUnknownFields: a newer snapshot has
	// fields this build does not know, and the useful error is the version.
	var probe struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("pricing snapshot: %w", err)
	}
	if probe.SchemaVersion < MinSchemaVersion || probe.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("pricing snapshot: schema_version %d not supported (this build reads %d to %d); upgrade costgate or regenerate with 'costgate pricing refresh'", probe.SchemaVersion, MinSchemaVersion, SchemaVersion)
	}
	var s Snapshot
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("pricing snapshot: %w", err)
	}
	if s.Currency != "USD" {
		return nil, fmt.Errorf("pricing snapshot: currency %q not supported", s.Currency)
	}
	if got := s.Digest(); got != s.SHA256 {
		return nil, fmt.Errorf("pricing snapshot: sha256 mismatch (file says %s, content is %s); regenerate with 'costgate pricing refresh'", s.SHA256, got)
	}
	s.parsed = make(map[string]*big.Rat, len(s.Prices))
	for k, p := range s.Prices {
		r, err := money.Parse(p.USD)
		if err != nil {
			return nil, fmt.Errorf("pricing snapshot: price %s: %w", k, err)
		}
		for _, t := range p.Tiers {
			if _, err := money.Parse(t.USD); err != nil {
				return nil, fmt.Errorf("pricing snapshot: price %s tier %s: %w", k, t.Start, err)
			}
		}
		s.parsed[k] = r
	}
	return &s, nil
}

// Lookup returns the unit price for key.
func (s *Snapshot) Lookup(key string) (*big.Rat, bool) {
	r, ok := s.parsed[key]
	return r, ok
}

// Info returns the billing metadata of key (SKU, rate code, raw usage type,
// description). Fields added in schema_version 2 are empty for a v1 snapshot.
func (s *Snapshot) Info(key string) Price {
	return s.Prices[key]
}
