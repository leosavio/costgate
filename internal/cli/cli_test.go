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
	docs := strings.Split(strings.TrimSuffix(string(jl), "\n"), "\n")
	if len(jl) == 0 {
		docs = nil
	}
	if len(docs) != len(want) {
		t.Fatalf("json lines: %d documents, report has %d cost lines", len(docs), len(want))
	}
	billingRules := map[string]bool{"fixed_hourly": true, "fixed_monthly": true, "usage": true}
	for i, d := range docs {
		var doc map[string]any
		if err := json.Unmarshal([]byte(d), &doc); err != nil {
			t.Fatalf("json lines %d: %v", i+1, err)
		}
		if doc["schema_version"] != float64(rep.SchemaVersion) || doc["version"] != rep.Version ||
			doc["pricing_as_of"] != rep.Pricing.AsOf || doc["kind"] != "costgate.line" {
			t.Errorf("json lines %d: report fields missing: %s", i+1, d)
		}
		for k, v := range want[i] {
			if doc[k] != v {
				t.Errorf("json lines %d: %s = %v, report line has %v", i+1, k, doc[k], v)
			}
		}
		for _, k := range []string{"sku", "usage_type", "rate_code", "billing_description"} {
			if s, _ := doc[k].(string); s == "" {
				t.Errorf("json lines %d: %s empty", i+1, k)
			}
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
		"region mismatch":  {"diff", "--plan", plan, "--region", "us-west-2"},
		"refresh region":   {"pricing", "refresh", "--region", "sa-east-1", "--out", filepath.Join(tmp, "x.json")},
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

func TestEmbeddedSnapshotVerifies(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"pricing", "verify"}, Env{Version: "test", Stdout: &stdout, Stderr: &stderr}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
}
