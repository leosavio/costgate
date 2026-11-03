// Package usage reads the usage premises file: values the plan cannot carry
// (GB per month, hours per month, Spot discount...).
package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"

	"github.com/leosavio/costgate/internal/money"
)

// SchemaVersion is the premises format this build reads.
const SchemaVersion = 1

// Entry holds optional premises. A nil field falls through to the next level.
type Entry struct {
	HoursPerMonth  *json.Number `json:"hours_per_month,omitempty"`
	GBPerMonth     *json.Number `json:"gb_per_month,omitempty"`
	SpotDiscount   *json.Number `json:"spot_discount,omitempty"`
	EventsPerMonth *json.Number `json:"events_per_month,omitempty"`
	AZCount        *json.Number `json:"az_count,omitempty"`
	RootVolumeGB   *json.Number `json:"root_volume_gb,omitempty"`
}

// File is the premises document.
type File struct {
	SchemaVersion int              `json:"schema_version"`
	Comment       string           `json:"comment,omitempty"`
	Defaults      Entry            `json:"defaults"`
	Types         map[string]Entry `json:"types,omitempty"`
	Resources     map[string]Entry `json:"resources,omitempty"`
}

// Built-in defaults used when no level declares a value.
var (
	DefaultHours        = money.Int(730)
	DefaultSpotDiscount = money.Zero()
	DefaultAZCount      = money.Int(2)
	DefaultRootVolumeGB = money.Int(8)
)

// Value is a resolved premise and where it came from.
type Value struct {
	V        *big.Rat
	Declared bool   // false when the built-in default was used
	From     string // "resources[...]", "types[...]", "defaults" or "built-in"
}

// Resolved holds every premise for one resource address.
type Resolved struct {
	Hours, GB, SpotDiscount, Events, AZCount, RootVolumeGB Value
}

// Empty returns a premises file with no declared value.
func Empty() *File { return &File{SchemaVersion: SchemaVersion} }

// Load decodes and validates a premises file.
func Load(data []byte) (*File, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("usage file: %w", err)
	}
	if f.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("usage file: schema_version %d not supported (this build reads %d)", f.SchemaVersion, SchemaVersion)
	}
	if err := validate("defaults", f.Defaults); err != nil {
		return nil, err
	}
	for k, e := range f.Types {
		if err := validate("types["+k+"]", e); err != nil {
			return nil, err
		}
	}
	for k, e := range f.Resources {
		if err := validate("resources["+k+"]", e); err != nil {
			return nil, err
		}
	}
	return &f, nil
}

func validate(where string, e Entry) error {
	check := func(name string, n *json.Number, lo, hi *big.Rat, hiOpen, integer bool) error {
		if n == nil {
			return nil
		}
		r, err := money.Parse(n.String())
		if err != nil {
			return fmt.Errorf("usage file: %s.%s: %w", where, name, err)
		}
		if r.Cmp(lo) < 0 || (hi != nil && (r.Cmp(hi) > 0 || (hiOpen && r.Cmp(hi) == 0))) {
			return fmt.Errorf("usage file: %s.%s = %s out of range", where, name, n.String())
		}
		if integer && !r.IsInt() {
			return fmt.Errorf("usage file: %s.%s = %s must be an integer", where, name, n.String())
		}
		return nil
	}
	zero, one := money.Zero(), money.Int(1)
	if err := check("hours_per_month", e.HoursPerMonth, zero, money.Int(744), false, false); err != nil {
		return err
	}
	if err := check("gb_per_month", e.GBPerMonth, zero, nil, false, false); err != nil {
		return err
	}
	if err := check("spot_discount", e.SpotDiscount, zero, one, true, false); err != nil {
		return err
	}
	if err := check("events_per_month", e.EventsPerMonth, zero, nil, false, false); err != nil {
		return err
	}
	if err := check("az_count", e.AZCount, one, money.Int(16), false, true); err != nil {
		return err
	}
	return check("root_volume_gb", e.RootVolumeGB, one, nil, false, false)
}

var indexSuffix = regexp.MustCompile(`\[[^\]]*\]`)

// Resolve applies the precedence resources[address] > resources[address
// without instance keys] > types[type] > defaults > built-in.
func (f *File) Resolve(address, resourceType string) Resolved {
	type level struct {
		name string
		e    Entry
		ok   bool
	}
	bare := indexSuffix.ReplaceAllString(address, "")
	var levels []level
	if e, ok := f.Resources[address]; ok {
		levels = append(levels, level{"resources[" + address + "]", e, true})
	}
	if bare != address {
		if e, ok := f.Resources[bare]; ok {
			levels = append(levels, level{"resources[" + bare + "]", e, true})
		}
	}
	if e, ok := f.Types[resourceType]; ok {
		levels = append(levels, level{"types[" + resourceType + "]", e, true})
	}
	levels = append(levels, level{"defaults", f.Defaults, true})

	pick := func(get func(Entry) *json.Number, def *big.Rat) Value {
		for _, l := range levels {
			if n := get(l.e); n != nil {
				return Value{V: money.MustParse(n.String()), Declared: true, From: l.name}
			}
		}
		if def == nil {
			return Value{V: money.Zero(), Declared: false, From: "not declared"}
		}
		return Value{V: def, Declared: false, From: "built-in"}
	}
	return Resolved{
		Hours:        pick(func(e Entry) *json.Number { return e.HoursPerMonth }, DefaultHours),
		GB:           pick(func(e Entry) *json.Number { return e.GBPerMonth }, nil),
		SpotDiscount: pick(func(e Entry) *json.Number { return e.SpotDiscount }, DefaultSpotDiscount),
		Events:       pick(func(e Entry) *json.Number { return e.EventsPerMonth }, nil),
		AZCount:      pick(func(e Entry) *json.Number { return e.AZCount }, DefaultAZCount),
		RootVolumeGB: pick(func(e Entry) *json.Number { return e.RootVolumeGB }, DefaultRootVolumeGB),
	}
}
