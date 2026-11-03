// Package report evaluates the gate and renders the estimate as Markdown
// (pull request comment) and JSON.
package report

import (
	"math/big"
	"strconv"

	"github.com/leosavio/costgate/internal/estimate"
)

// Exit codes. Usage and input errors (3) are produced by the CLI.
const (
	ExitOK          = 0
	ExitOverCeiling = 1
	ExitUnpriced    = 2
	ExitUsage       = 3
)

// Status values in the JSON report.
const (
	StatusOK          = "ok"
	StatusOverCeiling = "over_ceiling"
	StatusUnpriced    = "unpriced"
)

// Ceilings are the optional limits in USD per month.
type Ceilings struct {
	MaxDelta *big.Rat
	MaxTotal *big.Rat
}

// Verdict is the gate outcome.
type Verdict struct {
	Status   string
	ExitCode int
	Reasons  []string
}

// Evaluate applies the gate. Precedence: unpriced (2) over ceiling (1),
// because an incomplete estimate cannot prove the ceiling holds.
func Evaluate(res *estimate.Result, c Ceilings) Verdict {
	var unpricedReasons, ceilingReasons []string
	if n := len(res.Blocking()); n > 0 {
		unpricedReasons = append(unpricedReasons, plural(n, "resource created or changed has no known price", "resources created or changed have no known price"))
	}
	if c.MaxDelta != nil && res.Delta.Cmp(c.MaxDelta) > 0 {
		ceilingReasons = append(ceilingReasons, "monthly delta above --max-monthly-delta")
	}
	if c.MaxTotal != nil {
		if res.Incomplete() {
			unpricedReasons = append(unpricedReasons, "--max-monthly-total is set and the total is incomplete (unpriced resources)")
		}
		if res.After.Cmp(c.MaxTotal) > 0 {
			ceilingReasons = append(ceilingReasons, "monthly total after the change above --max-monthly-total")
		}
	}
	switch {
	case len(unpricedReasons) > 0:
		return Verdict{StatusUnpriced, ExitUnpriced, append(unpricedReasons, ceilingReasons...)}
	case len(ceilingReasons) > 0:
		return Verdict{StatusOverCeiling, ExitOverCeiling, ceilingReasons}
	}
	return Verdict{Status: StatusOK, ExitCode: ExitOK}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
