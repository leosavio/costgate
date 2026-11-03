package estimate

import (
	"encoding/json"
	"math/big"

	"github.com/leosavio/costgate/internal/money"
)

// attrs wraps one side (before or after) of a resource change. unknown is
// the matching after_unknown tree; it is nil for the before side.
type attrs struct {
	v       map[string]any
	unknown map[string]any
}

func newAttrs(v, unknown any) attrs {
	a := attrs{}
	a.v, _ = v.(map[string]any)
	a.unknown, _ = unknown.(map[string]any)
	return a
}

// isUnknown reports whether key is (entirely or partly) unknown at plan time.
func (a attrs) isUnknown(key string) bool {
	switch u := a.unknown[key].(type) {
	case bool:
		return u
	case nil:
		return false
	default:
		return true
	}
}

// wholeUnknown reports whether the whole value of key is unknown.
func (a attrs) wholeUnknown(key string) bool {
	b, ok := a.unknown[key].(bool)
	return ok && b
}

func (a attrs) str(key string) (string, bool) {
	s, ok := a.v[key].(string)
	return s, ok
}

func (a attrs) boolean(key string) (bool, bool) {
	b, ok := a.v[key].(bool)
	return b, ok
}

func (a attrs) list(key string) ([]any, bool) {
	l, ok := a.v[key].([]any)
	return l, ok
}

// first returns the first nested block of key as attrs, with its unknown tree.
func (a attrs) first(key string) (attrs, bool) {
	l, ok := a.list(key)
	if !ok || len(l) == 0 {
		return attrs{}, false
	}
	var u any
	if ul, ok := a.unknown[key].([]any); ok && len(ul) > 0 {
		u = ul[0]
	}
	return newAttrs(l[0], u), true
}

func numberOf(v any) (*big.Rat, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return nil, false
	}
	r, err := money.Parse(n.String())
	if err != nil {
		return nil, false
	}
	return r, true
}

func (a attrs) number(key string) (*big.Rat, bool) {
	return numberOf(a.v[key])
}
