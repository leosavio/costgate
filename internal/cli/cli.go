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
}

const usageText = `costgate projects the monthly AWS cost of a Terraform/OpenTofu plan.

Usage:
  costgate diff --plan <plan.json|-> [flags]
  costgate pricing refresh [--region us-east-1] [--out pricing/us-east-1.json]
  costgate pricing verify [--pricing <file>] [--region <region>]
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
	pricingPath := fs.String("pricing", "", "price snapshot file (default: embedded snapshot of --region)")
	region := fs.String("region", "us-east-1", "AWS region the plan deploys to")
	format := fs.String("format", "markdown", "stdout format: markdown or json")
	jsonOut := fs.String("json-out", "", "also write the JSON report to this file")
	jsonLinesOut := fs.String("json-lines", "", "also write one JSON document per cost line to this file (JSON Lines, for Athena or jq)")
	mdOut := fs.String("markdown-out", "", "also write the Markdown report to this file")
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
	snap, err := loadSnapshot(*pricingPath, *region)
	if err != nil {
		return fail(env, err)
	}
	res, err := estimate.Estimate(p, snap, use, estimate.Options{Region: *region, IgnoreProviders: ignore.sorted()})
	if err != nil {
		return fail(env, err)
	}
	ceil := report.Ceilings{MaxDelta: maxDelta.v, MaxTotal: maxTotal.v}
	in := report.Input{Version: env.Version, Result: res, Verdict: report.Evaluate(res, ceil), Ceilings: ceil, Pricing: snap,
		Warnings: report.RegionWarnings(res, pricing.Has)}

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
	region := fs.String("region", "us-east-1", "region to extract")
	out := fs.String("out", "", "output file (default pricing/<region>.json)")
	baseURL := fs.String("base-url", catalog.DefaultBaseURL, "Price List Bulk API base URL")
	timeout := fs.Duration("timeout", 20*time.Minute, "overall timeout")
	if err := fs.Parse(args); err != nil {
		return report.ExitUsage
	}
	if *out == "" {
		*out = filepath.Join("pricing", *region+".json")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	f := catalog.Fetcher{BaseURL: *baseURL, Client: env.HTTPClient, UserAgent: "costgate/" + env.Version}
	snap, err := catalog.Refresh(ctx, f, *region, catalog.Rules)
	if err != nil {
		return fail(env, err)
	}
	data, err := snap.Encode()
	if err != nil {
		return fail(env, err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Stdout, "wrote %s: %d prices, as of %s, sha256 %s\n", *out, len(snap.Prices), snap.AsOf, snap.SHA256)
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
