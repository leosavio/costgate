// Package money holds exact decimal arithmetic for prices and costs.
//
// Every amount is a *big.Rat so that results are identical byte for byte on
// every platform: float64 arithmetic may be fused (FMA) differently on arm64
// and amd64, which would break the determinism contract of the report.
package money

import (
	"fmt"
	"math/big"
	"strings"
)

// Zero returns a new zero amount.
func Zero() *big.Rat { return new(big.Rat) }

// Int returns n as an amount.
func Int(n int64) *big.Rat { return new(big.Rat).SetInt64(n) }

// Parse reads a decimal string such as "0.0450000000" or "1e3".
func Parse(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty number")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("invalid number %q", s)
	}
	return r, nil
}

// MustParse is Parse for compile-time constants.
func MustParse(s string) *big.Rat {
	r, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return r
}

// Add returns a+b without modifying the operands.
func Add(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }

// Sub returns a-b without modifying the operands.
func Sub(a, b *big.Rat) *big.Rat { return new(big.Rat).Sub(a, b) }

// Mul returns the product of all factors.
func Mul(factors ...*big.Rat) *big.Rat {
	out := new(big.Rat).SetInt64(1)
	for _, f := range factors {
		out.Mul(out, f)
	}
	return out
}

// Fixed formats r with exactly n decimals (halves rounded away from zero).
func Fixed(r *big.Rat, n int) string {
	s := r.FloatString(n)
	if strings.HasPrefix(s, "-") && strings.Trim(s, "-0.") == "" {
		return s[1:]
	}
	return s
}

// Signed formats r with n decimals and an explicit sign ("+1.00", "-2.50", "0.00").
func Signed(r *big.Rat, n int) string {
	s := Fixed(r, n)
	if strings.Trim(s, "0.") == "" {
		return s
	}
	if r.Sign() > 0 {
		return "+" + s
	}
	return s
}

// Trim formats r with up to 10 decimals and no trailing zeros ("0.045", "730").
func Trim(r *big.Rat) string {
	s := Fixed(r, 10)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}
