// Package cli implements the costgate command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/leosavio/costgate/internal/catalog"
	"github.com/leosavio/costgate/internal/estimate"
	"github.com/leosavio/costgate/internal/money"
	"github.com/leosavio/costgate/internal/plan"
	"github.com/leosavio/costgate/internal/report"
	"github.com/leosavio/costgate/internal/usage"
	"github.com/leosavio/costgate/pricing"
)

// Env carries the process boundary so tests can run the CLI in-process.
type Env struct {
	Version string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	// HTTPClient is used by `pricing refresh`; nil means a default client.
	HTTPClient *http.Client
	// Embedded returns the built-in snapshot of a region, false when there
	// is none; nil means the snapshots embedded in the binary. Tests set it
	// so that the reports do not depend on the weekly price refresh.
	Embedded func(region string) ([]byte, bool)
}

func (env Env) embedded(region string) ([]byte, bool) {
	if env.Embedded != nil {
		return env.Embedded(region)
	}
	if !pricing.Has(region) {
		return nil, false
	}
	b, err := pricing.Snapshot(region)
	return b, err == nil
}

const usageText = `costgate projects the monthly AWS cost of a Terraform/OpenTofu plan.

Usage:
  costgate diff --plan <plan.json|-> [flags]
  costgate pricing refresh [--region us-east-1] [--out pricing/us-east-1.json.gz] [--strict]
  costgate pricing verify [--pricing <file>] [--region <region>]
  costgate pricing regions
  costgate version

Exit codes: 0 ok, 1 above a ceiling, 2 unpriced resource, 3 usage or input error.
Run 'costgate <command> -h' for the flags of a command.
`

// Run executes the command line and returns the exit code.
func Run(args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usageText)
		return report.ExitUsage
	}
	switch args[0] {
	case "diff":
		return runDiff(args[1:], env)
	case "pricing":
		if len(args) < 2 {
			fmt.Fprint(env.Stderr, usageText)
			return report.ExitUsage
		}
		switch args[1] {
		case "refresh":
			return runRefresh(args[2:], env)
		case "verify":
			return runVerify(args[2:], env)
		case "regions":
			return runRegions(args[2:], env)
		}
	case "version", "--version":
		fmt.Fprintln(env.Stdout, "costgate", env.Version)
		return report.ExitOK
	case "help", "-h", "--help":
		fmt.Fprint(env.Stdout, usageText)
		return report.ExitOK
	}
	fmt.Fprintf(env.Stderr, "costgate: unknown command %q\n\n%s", args[0], usageText)
	return report.ExitUsage
}

func fail(env Env, err error) int {
	fmt.Fprintln(env.Stderr, "costgate:", err)
	return report.ExitUsage
}

type ratFlag struct{ v *big.Rat }

func (f *ratFlag) String() string {
	if f.v == nil {
		return ""
	}
	return money.Trim(f.v)
}

func (f *ratFlag) Set(s string) error {
	r, err := money.Parse(s)
	if err != nil {
		return err
	}
	if r.Sign() < 0 {
		return errors.New("must not be negative")
	}
	f.v = r
	return nil
}

// providerList is a repeatable, comma-separated list of providers.
type providerList struct{ v []string }

func (p *providerList) String() string { return strings.Join(p.v, ",") }

func (p *providerList) Set(s string) error {
	for _, e := range strings.Split(s, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			return errors.New("empty provider name")
		}
		if e == "aws" || strings.HasSuffix(e, "/aws") {
			return errors.New("the AWS provider cannot be ignored; costgate prices it")
		}
		p.v = append(p.v, e)
	}
	return nil
}

// sorted returns the entries sorted and without duplicates, so the reports
// do not depend on the flag order.
func (p *providerList) sorted() []string {
	out := append([]string{}, p.v...)
	sort.Strings(out)
	return slices.Compact(out)
}

// pathList is a repeatable flag of file paths.
type pathList struct{ v []string }

func (p *pathList) String() string { return strings.Join(p.v, ",") }

func (p *pathList) Set(s string) error {
	if s == "" {
		return errors.New("empty path")
	}
	p.v = append(p.v, s)
	return nil
}

// snapshotSet holds the --pricing files by region and loads the embedded
// snapshot of any other region on first use.
type snapshotSet struct {
	env   Env
	files map[string]*catalog.Snapshot
}

func loadSnapshotFiles(env Env, paths []string) (*snapshotSet, error) {
	set := &snapshotSet{env: env, files: map[string]*catalog.Snapshot{}}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		snap, err := catalog.Load(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if _, dup := set.files[snap.Region]; dup {
			return nil, fmt.Errorf("--pricing: two snapshots for region %s", snap.Region)
		}
		set.files[snap.Region] = snap
	}
	return set, nil
}

// prices is the estimate.Prices of the set: a --pricing file, else the
// embedded snapshot, else none.
func (s *snapshotSet) prices(region string) (*catalog.Snapshot, error) {
	if snap, ok := s.files[region]; ok {
		return snap, nil
	}
	data, ok := s.env.embedded(region)
	if !ok {
		return nil, nil
	}
	snap, err := catalog.Load(data)
	if err != nil {
		return nil, fmt.Errorf("embedded snapshot of %s: %w", region, err)
	}
	return snap, nil
}

// defaultRegion picks the region of the resources whose region the plan
// does not state, the caller's choice first: --region; else the region of
// the only --pricing file; else the only region the plan names, when
// costgate has its prices; else us-east-1.
func (s *snapshotSet) defaultRegion(p *plan.Plan, flagRegion string) (string, string, error) {
	if flagRegion != "" {
		return flagRegion, estimate.RegionFromFlag, nil
	}
	if len(s.files) == 1 {
		for r := range s.files {
			return r, estimate.RegionFromFlag, nil
		}
	}
	if rs := estimate.PlanRegions(p); len(rs) == 1 {
		snap, err := s.prices(rs[0])
		if err != nil {
			return "", "", err
		}
		if snap != nil {
			return rs[0], estimate.RegionFromPlan, nil
		}
	}
	return estimate.DefaultRegion, estimate.RegionFromDefault, nil
}

func noSnapshotError(region string) error {
	if catalog.RegionSupported(region) {
		return fmt.Errorf("no price snapshot for region %[1]s; run 'costgate pricing refresh --region %[1]s --out %[1]s.json.gz' and pass --pricing %[1]s.json.gz", region)
	}
	return fmt.Errorf("no price snapshot for region %s: the public AWS Price List of this endpoint has no such region (see 'costgate pricing regions'); pass --pricing <snapshot of %s>", region, region)
}

func loadSnapshot(path, region string) (*catalog.Snapshot, error) {
	var data []byte
	var err error
	if path == "" {
		data, err = pricing.Snapshot(region)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	snap, err := catalog.Load(data)
	if err != nil {
		return nil, err
	}
	if snap.Region != region {
		return nil, fmt.Errorf("pricing snapshot is for %s, --region is %s", snap.Region, region)
	}
	return snap, nil
}

func runDiff(args []string, env Env) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	planPath := fs.String("plan", "", "plan JSON from 'terraform show -json <planfile>' ('-' for stdin)")
	usagePath := fs.String("usage", "", "usage premises file (JSON, schema_version 1)")
	var pricingPaths pathList
	fs.Var(&pricingPaths, "pricing", "price snapshot file, JSON or .json.gz, used instead of the embedded one of its region (repeatable)")
	region := fs.String("region", "", "region of the resources whose region the plan does not state (default: the only region the plan names, else us-east-1); every other resource is priced in its own region")
	format := fs.String("format", "markdown", "stdout format: markdown or json")
	jsonOut := fs.String("json-out", "", "also write the JSON report to this file")
	jsonLinesOut := fs.String("json-lines", "", "also write one JSON document per cost line to this file (JSON Lines, for Athena or jq)")
	mdOut := fs.String("markdown-out", "", "also write the Markdown report to this file")
	htmlOut := fs.String("html-out", "", "also write a self-contained HTML report to this file (inline data, no network)")
	var maxDelta, maxTotal ratFlag
	fs.Var(&maxDelta, "max-monthly-delta", "fail (exit 1) when the monthly delta in USD is above this value")
	fs.Var(&maxTotal, "max-monthly-total", "fail (exit 1) when the monthly total after the change is above this value")
	var ignore providerList
	fs.Var(&ignore, "ignore-provider", "comma-separated providers (helm,kubectl,kubernetes) whose resources are left out of the estimate and the gate, and listed as ignored")
	if err := fs.Parse(args); err != nil {
		return report.ExitUsage
	}
	if fs.NArg() > 0 {
		return fail(env, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	if *planPath == "" {
		return fail(env, errors.New("--plan is required"))
	}
	if *format != "markdown" && *format != "json" {
		return fail(env, fmt.Errorf("--format %q: want markdown or json", *format))
	}

	var planReader io.Reader
	if *planPath == "-" {
		planReader = env.Stdin
	} else {
		f, err := os.Open(*planPath)
		if err != nil {
			return fail(env, err)
		}
		defer f.Close()
		planReader = f
	}
	p, err := plan.Parse(planReader)
	if err != nil {
		return fail(env, err)
	}
	use := usage.Empty()
	if *usagePath != "" {
		data, err := os.ReadFile(*usagePath)
		if err != nil {
			return fail(env, err)
		}
		if use, err = usage.Load(data); err != nil {
			return fail(env, err)
		}
	}
	snaps, err := loadSnapshotFiles(env, pricingPaths.v)
	if err != nil {
		return fail(env, err)
	}
	defRegion, regionFrom, err := snaps.defaultRegion(p, *region)
	if err != nil {
		return fail(env, err)
	}
	defSnap, err := snaps.prices(defRegion)
	if err != nil {
		return fail(env, err)
	}
	if defSnap == nil {
		return fail(env, noSnapshotError(defRegion))
	}
	res, err := estimate.Estimate(p, snaps.prices, use, estimate.Options{Region: defRegion, RegionFrom: regionFrom, IgnoreProviders: ignore.sorted()})
	if err != nil {
		return fail(env, err)
	}
	ceil := report.Ceilings{MaxDelta: maxDelta.v, MaxTotal: maxTotal.v}
	in := report.Input{Version: env.Version, Result: res, Verdict: report.Evaluate(res, ceil), Ceilings: ceil,
		Pricing: defSnap, Warnings: report.RegionWarnings(res)}

	md := report.Markdown(in)
	js, err := report.JSON(in)
	if err != nil {
		return fail(env, err)
	}
	if *jsonOut != "" {
		if err := os.WriteFile(*jsonOut, js, 0o644); err != nil {
			return fail(env, err)
		}
	}
	if *jsonLinesOut != "" {
		jl, err := report.JSONLines(in)
		if err != nil {
			return fail(env, err)
		}
		if err := os.WriteFile(*jsonLinesOut, jl, 0o644); err != nil {
			return fail(env, err)
		}
	}
	if *mdOut != "" {
		if err := os.WriteFile(*mdOut, []byte(md), 0o644); err != nil {
			return fail(env, err)
		}
	}
	if *htmlOut != "" {
		page, err := report.HTML(in)
		if err != nil {
			return fail(env, err)
		}
		if err := os.WriteFile(*htmlOut, page, 0o644); err != nil {
			return fail(env, err)
		}
	}
	if *format == "json" {
		_, err = env.Stdout.Write(js)
	} else {
		_, err = io.WriteString(env.Stdout, md)
	}
	if err != nil {
		return fail(env, err)
	}
	return in.Verdict.ExitCode
}

func runRefresh(args []string, env Env) int {
	fs := flag.NewFlagSet("pricing refresh", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	region := fs.String("region", "us-east-1", "region to extract (see 'costgate pricing regions')")
	out := fs.String("out", "", "output file, gzip when it ends in .gz (default pricing/<region>.json.gz)")
	baseURL := fs.String("base-url", catalog.DefaultBaseURL, "Price List Bulk API base URL")
	timeout := fs.Duration("timeout", 20*time.Minute, "overall timeout")
	strict := fs.Bool("strict", false, "fail when a pricing rule has no price in the region (use on us-east-1 to detect an offer format change)")
	if err := fs.Parse(args); err != nil {
		return report.ExitUsage
	}
	if *out == "" {
		*out = filepath.Join("pricing", *region+".json.gz")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	f := catalog.Fetcher{BaseURL: *baseURL, Client: env.HTTPClient, UserAgent: "costgate/" + env.Version}
	snap, missing, err := catalog.Refresh(ctx, f, *region, catalog.Rules, *strict)
	if err != nil {
		return fail(env, err)
	}
	var data []byte
	if strings.HasSuffix(*out, ".gz") {
		data, err = snap.EncodeGzip()
	} else {
		data, err = snap.Encode()
	}
	if err != nil {
		return fail(env, err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return fail(env, err)
	}
	for _, m := range missing {
		fmt.Fprintf(env.Stderr, "costgate: %s: no on-demand price for rule %s; resources that need it stay unpriced\n", *region, m)
	}
	fmt.Fprintf(env.Stdout, "wrote %s: %d prices, as of %s, sha256 %s\n", *out, len(snap.Prices), snap.AsOf, snap.SHA256)
	return report.ExitOK
}

// runRegions prints the regions refresh supports and whether the binary
// embeds a snapshot of each.
func runRegions(args []string, env Env) int {
	fs := flag.NewFlagSet("pricing regions", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	if err := fs.Parse(args); err != nil {
		return report.ExitUsage
	}
	if fs.NArg() > 0 {
		return fail(env, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	for _, r := range catalog.Regions() {
		state := "refresh only"
		if pricing.Has(r) {
			state = "embedded"
		}
		fmt.Fprintf(env.Stdout, "%s\t%s\n", r, state)
	}
	return report.ExitOK
}

// runVerify checks a snapshot: every embedded one by default, the embedded
// one of --region, or the file of --pricing (whose region must match
// --region when both are given).
func runVerify(args []string, env Env) int {
	fs := flag.NewFlagSet("pricing verify", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	region := fs.String("region", "", "region of the snapshot (default: every embedded snapshot, or the region of --pricing)")
	path := fs.String("pricing", "", "snapshot file (default: embedded)")
	if err := fs.Parse(args); err != nil {
		return report.ExitUsage
	}
	if fs.NArg() > 0 {
		return fail(env, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	if *path != "" && *region == "" {
		data, err := os.ReadFile(*path)
		if err != nil {
			return fail(env, err)
		}
		snap, err := catalog.Load(data)
		if err != nil {
			return fail(env, err)
		}
		printVerified(env, snap)
		return report.ExitOK
	}
	regions := pricing.Regions()
	if *region != "" {
		regions = []string{*region}
	}
	code := report.ExitOK
	for _, r := range regions {
		snap, err := loadSnapshot(*path, r)
		if err != nil {
			fmt.Fprintf(env.Stderr, "costgate: %s: %v\n", r, err)
			code = report.ExitUsage
			continue
		}
		printVerified(env, snap)
	}
	return code
}

func printVerified(env Env, snap *catalog.Snapshot) {
	fmt.Fprintf(env.Stdout, "ok %s: %d prices, as of %s, sha256 %s\n", snap.Region, len(snap.Prices), snap.AsOf, snap.SHA256)
	for _, s := range snap.Sources {
		fmt.Fprintf(env.Stdout, "  %s/%s version %s published %s\n", s.Service, s.Scope, s.Version, s.PublicationDate)
	}
}
