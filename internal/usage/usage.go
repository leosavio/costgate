// Package usage reads the usage premises file: values the plan cannot carry
// (GB per month, hours per month, Spot discount...).
package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"

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
	// ExtendedSupportHours: hours per month an EKS cluster runs a
	// Kubernetes version in extended support. No version calendar is
	// embedded; this premise is the only source.
	ExtendedSupportHours *json.Number `json:"extended_support_hours,omitempty"`
	// AutoModeInstanceHours: instance hours per month of the nodes EKS
	// Auto Mode launches, by instance type ({"c6a.xlarge": 1460}). The
	// plan does not carry these nodes.
	AutoModeInstanceHours map[string]json.Number `json:"auto_mode_instance_hours,omitempty"`
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
	Hours, GB, SpotDiscount, Events, AZCount, RootVolumeGB, ExtendedSupportHours Value
	AutoModeInstanceHours                                                        MapValue
}

// MapValue is a resolved premise keyed by name (instance type), taken
// whole from the first level that declares it.
type MapValue struct {
	V        map[string]*big.Rat
	Declared bool
	From     string
}

// Keys returns the keys of V, sorted.
func (m MapValue) Keys() []string {
	out := make([]string, 0, len(m.V))
	for k := range m.V {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
	if err := check("extended_support_hours", e.ExtendedSupportHours, zero, money.Int(744), false, false); err != nil {
		return err
	}
	for _, it := range sortedKeys(e.AutoModeInstanceHours) {
		if !instanceTypePattern.MatchString(it) {
			return fmt.Errorf("usage file: %s.auto_mode_instance_hours: %q is not an instance type", where, it)
		}
		n := e.AutoModeInstanceHours[it]
		if err := check("auto_mode_instance_hours["+it+"]", &n, zero, nil, false, false); err != nil {
			return err
		}
	}
	return check("root_volume_gb", e.RootVolumeGB, one, nil, false, false)
}

var instanceTypePattern = regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9-]+$`)

func sortedKeys(m map[string]json.Number) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
	autoMode := MapValue{From: "not declared"}
	for _, l := range levels {
		if l.e.AutoModeInstanceHours == nil {
			continue
		}
		autoMode = MapValue{V: map[string]*big.Rat{}, Declared: true, From: l.name}
		for it, n := range l.e.AutoModeInstanceHours {
			autoMode.V[it] = money.MustParse(n.String())
		}
		break
	}
	return Resolved{
		Hours:                 pick(func(e Entry) *json.Number { return e.HoursPerMonth }, DefaultHours),
		GB:                    pick(func(e Entry) *json.Number { return e.GBPerMonth }, nil),
		SpotDiscount:          pick(func(e Entry) *json.Number { return e.SpotDiscount }, DefaultSpotDiscount),
		Events:                pick(func(e Entry) *json.Number { return e.EventsPerMonth }, nil),
		AZCount:               pick(func(e Entry) *json.Number { return e.AZCount }, DefaultAZCount),
		RootVolumeGB:          pick(func(e Entry) *json.Number { return e.RootVolumeGB }, DefaultRootVolumeGB),
		ExtendedSupportHours:  pick(func(e Entry) *json.Number { return e.ExtendedSupportHours }, nil),
		AutoModeInstanceHours: autoMode,
	}
}
