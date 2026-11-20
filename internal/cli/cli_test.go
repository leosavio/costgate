package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

const root = "../../testdata"

// fixtureSnapshot runs `pricing refresh` against the reduced Bulk API mirror
// in testdata/offers and returns the path of the written snapshot.
func fixtureSnapshot(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(root, "offers"))))
	t.Cleanup(srv.Close)
	out := filepath.Join(t.TempDir(), "us-east-1.json")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pricing", "refresh", "--base-url", srv.URL, "--out", out},
		Env{Version: "test", Stdout: &stdout, Stderr: &stderr, HTTPClient: srv.Client()})
	if code != 0 {
		t.Fatalf("refresh exit %d: %s", code, stderr.String())
	}
	return out
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/cli -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from output; run go test ./internal/cli -update and review the diff\n--- got ---\n%s", path, got)
	}
}

func TestRefreshFixtureGolden(t *testing.T) {
	got, err := os.ReadFile(fixtureSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	golden(t, filepath.Join(root, "pricing.golden.json"), got)
}

func runCase(t *testing.T, dir, snapshot string) (md, js, jl []byte, code int) {
	t.Helper()
	args := []string{"diff", "--plan", filepath.Join(dir, "plan.json"), "--pricing", snapshot}
	if _, err := os.Stat(filepath.Join(dir, "usage.json")); err == nil {
		args = append(args, "--usage", filepath.Join(dir, "usage.json"))
	}
	if extra, err := os.ReadFile(filepath.Join(dir, "args.txt")); err == nil {
		args = append(args, strings.Fields(string(extra))...)
	}
	jsonPath := filepath.Join(t.TempDir(), "report.json")
	linesPath := filepath.Join(t.TempDir(), "report.jsonl")
	args = append(args, "--json-out", jsonPath, "--json-lines", linesPath)
	var stdout, stderr bytes.Buffer
	code = Run(args, Env{Version: "test", Stdout: &stdout, Stderr: &stderr})
	if stderr.Len() > 0 {
		t.Fatalf("stderr: %s", stderr.String())
	}
	js, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	jl, err = os.ReadFile(linesPath)
	if err != nil {
		t.Fatal(err)
	}
	return stdout.Bytes(), js, jl, code
}

func TestGoldenCases(t *testing.T) {
	snapshot := fixtureSnapshot(t)
	dirs, err := filepath.Glob(filepath.Join(root, "cases", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases: %v", err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			md, js, jl, code := runCase(t, dir, snapshot)
			golden(t, filepath.Join(dir, "want.md"), md)
			golden(t, filepath.Join(dir, "want.json"), js)
			golden(t, filepath.Join(dir, "want.jsonl"), jl)
			golden(t, filepath.Join(dir, "want_exit"), []byte(strconv.Itoa(code)+"\n"))
			checkJSONLines(t, js, jl)

			// Determinism: a second run produces the same bytes.
			md2, js2, jl2, code2 := runCase(t, dir, snapshot)
			if !bytes.Equal(md, md2) || !bytes.Equal(js, js2) || !bytes.Equal(jl, jl2) || code != code2 {
				t.Error("second run differs from the first")
			}
		})
	}
}

// checkJSONLines asserts that every JSON Lines document is standalone and
// that the lines are exactly the cost lines of the JSON report.
func checkJSONLines(t *testing.T, js, jl []byte) {
	t.Helper()
	var rep struct {
		SchemaVersion int    `json:"schema_version"`
		Version       string `json:"version"`
		Pricing       struct {
			AsOf string `json:"as_of"`
		} `json:"pricing"`
		Warnings  []map[string]any `json:"warnings"`
		Ignored   []map[string]any `json:"ignored"`
		Resources []struct {
			Address string `json:"address"`
			Before  *struct {
				Lines []map[string]any `json:"lines"`
			} `json:"before"`
			After *struct {
				Lines []map[string]any `json:"lines"`
			} `json:"after"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(js, &rep); err != nil {
		t.Fatal(err)
	}
	var want []map[string]any
	for _, r := range rep.Resources {
		if r.Before != nil {
			want = append(want, r.Before.Lines...)
		}
		if r.After != nil {
			want = append(want, r.After.Lines...)
		}
	}
	all := strings.Split(strings.TrimSuffix(string(jl), "\n"), "\n")
	if len(jl) == 0 {
		all = nil
	}
	// Warnings first, cost lines, then ignored resources.
	byKind := map[string][]map[string]any{}
	var order []string
	for i, d := range all {
		var doc map[string]any
		if err := json.Unmarshal([]byte(d), &doc); err != nil {
			t.Fatalf("json lines %d: %v", i+1, err)
		}
		if doc["schema_version"] != float64(rep.SchemaVersion) || doc["version"] != rep.Version || doc["pricing_as_of"] != rep.Pricing.AsOf {
			t.Errorf("json lines %d: report fields missing: %s", i+1, d)
		}
		kind, _ := doc["kind"].(string)
		if len(order) == 0 || order[len(order)-1] != kind {
			order = append(order, kind)
		}
		byKind[kind] = append(byKind[kind], doc)
	}
	rank := map[string]int{"costgate.warning": 0, "costgate.line": 1, "costgate.ignored": 2}
	for i, k := range order {
		if _, ok := rank[k]; !ok || (i > 0 && rank[order[i-1]] >= rank[k]) {
			t.Errorf("json lines: kinds out of order or unknown: %v", order)
		}
	}
	if len(byKind["costgate.warning"]) != len(rep.Warnings) || len(byKind["costgate.ignored"]) != len(rep.Ignored) {
		t.Errorf("json lines: %d warnings and %d ignored, report has %d and %d",
			len(byKind["costgate.warning"]), len(byKind["costgate.ignored"]), len(rep.Warnings), len(rep.Ignored))
	}
	for i, w := range rep.Warnings {
		if i < len(byKind["costgate.warning"]) && byKind["costgate.warning"][i]["fix"] != w["fix"] {
			t.Errorf("json lines warning %d differs from the report", i+1)
		}
	}
	docs := byKind["costgate.line"]
	if len(docs) != len(want) {
		t.Fatalf("json lines: %d cost line documents, report has %d cost lines", len(docs), len(want))
	}
	billingRules := map[string]bool{"fixed_hourly": true, "fixed_monthly": true, "usage": true}
	for i, doc := range docs {
		for k, v := range want[i] {
			if doc[k] != v {
				t.Errorf("json lines %d: %s = %v, report line has %v", i+1, k, doc[k], v)
			}
		}
		// A premise-only line (undeclared, no price key) has no bill keys.
		if doc["price_key"] != "" {
			for _, k := range []string{"sku", "usage_type", "rate_code", "billing_description"} {
				if s, _ := doc[k].(string); s == "" {
					t.Errorf("json lines %d: %s empty", i+1, k)
				}
			}
		} else if doc["declared"] != false || doc["monthly"] != float64(0) {
			t.Errorf("json lines %d: line without price key must be an undeclared premise counted 0: %v", i+1, doc)
		}
		if r, _ := doc["billing_rule"].(string); !billingRules[r] {
			t.Errorf("json lines %d: billing_rule %q", i+1, r)
		}
	}
}

// TestV1SnapshotStillLoads prices a plan with a schema_version 1 snapshot
// written by costgate 0.1.x: same totals, empty v2 billing fields.
func TestV1SnapshotStillLoads(t *testing.T) {
	dir := filepath.Join(root, "cases", "baseline")
	_, jsV2, _, code := runCase(t, dir, fixtureSnapshot(t))
	_, jsV1, _, codeV1 := runCase(t, dir, filepath.Join(root, "pricing.v1.json"))
	if code != codeV1 {
		t.Fatalf("exit %d with v2, %d with v1", code, codeV1)
	}
	type totals struct {
		Totals map[string]json.Number `json:"totals"`
	}
	var a, b totals
	if err := json.Unmarshal(jsV2, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(jsV1, &b); err != nil {
		t.Fatal(err)
	}
	for k, v := range a.Totals {
		if b.Totals[k] != v {
			t.Errorf("%s: v2 %s, v1 %s", k, v, b.Totals[k])
		}
	}
	if !bytes.Contains(jsV1, []byte(`"rate_code": ""`)) || !bytes.Contains(jsV1, []byte(`"sku": "`)) {
		t.Error("v1 snapshot: want sku filled and rate_code empty")
	}
}

func TestUsageErrorsExit3(t *testing.T) {
	tmp := t.TempDir()
	badUsage := filepath.Join(tmp, "usage.json")
	if err := os.WriteFile(badUsage, []byte(`{"schema_version": 2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(tmp, "state.json")
	if err := os.WriteFile(statePath, []byte(`{"format_version":"1.0","values":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(root, "cases", "changes", "plan.json")
	cases := map[string][]string{
		"no command":       {},
		"unknown command":  {"estimate"},
		"missing plan":     {"diff"},
		"bad format":       {"diff", "--plan", plan, "--format", "yaml"},
		"negative ceiling": {"diff", "--plan", plan, "--max-monthly-delta", "-1"},
		"bad usage":        {"diff", "--plan", plan, "--usage", badUsage},
		"state not plan":   {"diff", "--plan", statePath},
		"missing file":     {"diff", "--plan", filepath.Join(tmp, "nope.json")},
		"not embedded":     {"diff", "--plan", plan, "--region", "us-east-2"},
		"snapshot region":  {"diff", "--plan", plan, "--region", "us-west-2", "--pricing", filepath.Join(root, "pricing.golden.json")},
		"refresh region":   {"pricing", "refresh", "--region", "sa-east-1", "--out", filepath.Join(tmp, "x.json")},
		"ignore aws":       {"diff", "--plan", plan, "--ignore-provider", "helm,aws"},
		"ignore hc aws":    {"diff", "--plan", plan, "--ignore-provider", "hashicorp/aws"},
		"ignore empty":     {"diff", "--plan", plan, "--ignore-provider", "helm,"},
		"verify mismatch":  {"pricing", "verify", "--pricing", filepath.Join(root, "pricing.golden.json"), "--region", "us-west-2"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(args, Env{Version: "test", Stdout: &stdout, Stderr: &stderr}); code != 3 {
				t.Errorf("exit %d, want 3; stderr: %s", code, stderr.String())
			}
		})
	}
}

func TestStdinPlanAndJSONFormat(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "cases", "changes", "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"diff", "--plan", "-", "--format", "json"},
		Env{Version: "test", Stdin: bytes.NewReader(data), Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schema_version": 1`) {
		t.Errorf("stdout is not the JSON report:\n%s", stdout.String())
	}
}

// pricing verify without flags checks every embedded snapshot.
func TestEmbeddedSnapshotVerifies(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"pricing", "verify"}, Env{Version: "test", Stdout: &stdout, Stderr: &stderr}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, r := range []string{"us-east-1", "us-west-2"} {
		if !strings.Contains(stdout.String(), "ok "+r+": ") {
			t.Errorf("verify did not check %s:\n%s", r, stdout.String())
		}
	}
	stdout.Reset()
	if code := Run([]string{"pricing", "verify", "--region", "us-west-2"}, Env{Version: "test", Stdout: &stdout, Stderr: &stderr}); code != 0 ||
		strings.Contains(stdout.String(), "us-east-1:") {
		t.Errorf("verify --region us-west-2: exit %d\n%s", code, stdout.String())
	}
	stdout.Reset()
	if code := Run([]string{"pricing", "verify", "--pricing", filepath.Join(root, "pricing.golden.json")}, Env{Version: "test", Stdout: &stdout, Stderr: &stderr}); code != 0 {
		t.Errorf("verify --pricing: exit %d: %s", code, stderr.String())
	}
}

// --region us-west-2 works without --pricing: the snapshot is embedded.
func TestEmbeddedUSWest2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"diff", "--plan", filepath.Join(root, "cases", "region-provider", "plan.json"), "--region", "us-west-2", "--format", "json"},
		Env{Version: "test", Stdout: &stdout, Stderr: &stderr})
	if code != 0 && code != 2 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"region": "us-west-2"`) || !strings.Contains(out, `"usage_type": "USW2-PublicIPv4:InUseAddress"`) ||
		strings.Contains(out, "USE1-") || strings.Contains(out, `"code": "region_mismatch"`) {
		t.Errorf("us-west-2 run does not price with USW2 keys or still warns:\n%s", out)
	}
}
