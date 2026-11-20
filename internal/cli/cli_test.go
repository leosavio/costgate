package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	code := Run([]string{"pricing", "refresh", "--strict", "--base-url", srv.URL, "--out", out},
		Env{Version: "test", Stdout: &stdout, Stderr: &stderr, HTTPClient: srv.Client()})
	if code != 0 {
		t.Fatalf("refresh exit %d: %s", code, stderr.String())
	}
	return out
}

// TestRefreshGzipIsDeterministic: the .json.gz form is the JSON form,
// compressed, and the same bytes on every run.
func TestRefreshGzipIsDeterministic(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(root, "offers"))))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	var outs [][]byte
	for _, name := range []string{"a.json.gz", "b.json.gz"} {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"pricing", "refresh", "--base-url", srv.URL, "--out", filepath.Join(dir, name)},
			Env{Version: "test", Stdout: &stdout, Stderr: &stderr, HTTPClient: srv.Client()}); code != 0 {
			t.Fatalf("refresh exit %d: %s", code, stderr.String())
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		outs = append(outs, data)
	}
	if !bytes.Equal(outs[0], outs[1]) || outs[0][0] != 0x1f || outs[0][1] != 0x8b {
		t.Fatal("gzip snapshot differs between runs or is not gzip")
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"pricing", "verify", "--pricing", filepath.Join(dir, "a.json.gz")}, Env{Version: "test", Stdout: &stdout, Stderr: &stderr}); code != 0 {
		t.Fatalf("verify gzip: exit %d: %s", code, stderr.String())
	}
	golden, err := os.ReadFile(filepath.Join(root, "pricing.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sha256 "+jsonField(t, golden, "sha256")) {
		t.Errorf("gzip snapshot has another digest than the JSON golden:\n%s", stdout.String())
	}
}

func jsonField(t *testing.T, data []byte, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	s, _ := m[key].(string)
	return s
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

type caseOutput struct {
	md, js, jl, html []byte
	code             int
}

// runCase runs a golden case with only the us-east-1 fixture snapshot.
func runCase(t *testing.T, dir, snapshot string) caseOutput {
	t.Helper()
	return runCaseEnv(t, dir, snapshot, Env{Version: "test", Embedded: func(string) ([]byte, bool) { return nil, false }})
}

// runCaseEnv runs a golden case with env's built-in snapshots. The case
// passes --pricing snapshot unless its directory has a file named
// "embedded": then the plan alone picks the regions, as without flags.
func runCaseEnv(t *testing.T, dir, snapshot string, env Env) caseOutput {
	t.Helper()
	args := []string{"diff", "--plan", filepath.Join(dir, "plan.json")}
	if _, err := os.Stat(filepath.Join(dir, "embedded")); err != nil {
		args = append(args, "--pricing", snapshot)
	}
	if _, err := os.Stat(filepath.Join(dir, "usage.json")); err == nil {
		args = append(args, "--usage", filepath.Join(dir, "usage.json"))
	}
	if extra, err := os.ReadFile(filepath.Join(dir, "args.txt")); err == nil {
		args = append(args, strings.Fields(string(extra))...)
	}
	tmp := t.TempDir()
	jsonPath := filepath.Join(tmp, "report.json")
	linesPath := filepath.Join(tmp, "report.jsonl")
	htmlPath := filepath.Join(tmp, "report.html")
	args = append(args, "--json-out", jsonPath, "--json-lines", linesPath, "--html-out", htmlPath)
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr
	out := caseOutput{code: Run(args, env)}
	if stderr.Len() > 0 {
		t.Fatalf("stderr: %s", stderr.String())
	}
	out.md = stdout.Bytes()
	var err error
	for path, dst := range map[string]*[]byte{jsonPath: &out.js, linesPath: &out.jl, htmlPath: &out.html} {
		if *dst, err = os.ReadFile(path); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestGoldenCases(t *testing.T) {
	snapshot, west := fixtureSnapshots(t)
	env := fixtureEnv(t, snapshot, west)
	dirs, err := filepath.Glob(filepath.Join(root, "cases", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases: %v", err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			o := runCaseEnv(t, dir, snapshot, env)
			golden(t, filepath.Join(dir, "want.md"), o.md)
			golden(t, filepath.Join(dir, "want.json"), o.js)
			golden(t, filepath.Join(dir, "want.jsonl"), o.jl)
			golden(t, filepath.Join(dir, "want.html"), o.html)
			golden(t, filepath.Join(dir, "want_exit"), []byte(strconv.Itoa(o.code)+"\n"))
			checkJSONLines(t, o.js, o.jl)
			checkHTML(t, o.html, o.js, o.jl)

			// Determinism: a second run produces the same bytes.
			o2 := runCaseEnv(t, dir, snapshot, env)
			if !bytes.Equal(o.md, o2.md) || !bytes.Equal(o.js, o2.js) || !bytes.Equal(o.jl, o2.jl) ||
				!bytes.Equal(o.html, o2.html) || o.code != o2.code {
				t.Error("second run differs from the first")
			}
		})
	}
}

const dataOpen, scriptClose = `<script type="application/json" id="costgate-data">`, "</script>"

// pageData returns the JSON embedded in an HTML report.
func pageData(t *testing.T, page []byte) []byte {
	t.Helper()
	_, rest, ok := bytes.Cut(page, []byte(dataOpen))
	if !ok {
		t.Fatal("html: no data element")
	}
	data, _, ok := bytes.Cut(rest, []byte(scriptClose))
	if !ok {
		t.Fatal("html: data element not closed")
	}
	return data
}

func decodeNumbers(t *testing.T, data []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		t.Fatal(err)
	}
}

// checkHTML asserts that the page is self-contained, that it embeds the JSON
// report unchanged, and that its export matches the JSON Lines output.
func checkHTML(t *testing.T, page, js, jl []byte) {
	t.Helper()
	// The page without its data: nothing in it may load a resource.
	shell := bytes.Replace(page, pageData(t, page), nil, 1)
	for _, s := range []string{"__CSP__", "__VERSION__", "__CSS__", "__JS__", "__DATA__", "unsafe-inline",
		"http://", "https://", "src=", "<link", "<iframe", "@import", "url("} {
		if bytes.Contains(shell, []byte(s)) {
			t.Errorf("html contains %q", s)
		}
	}
	if !bytes.Contains(page, []byte(`content="default-src 'none'; script-src 'sha256-`)) {
		t.Error("html: no Content-Security-Policy with default-src 'none'")
	}
	var data struct {
		Report json.RawMessage `json:"report"`
		View   struct {
			Premises []struct {
				Resource int    `json:"resource"`
				PerUnit  string `json:"per_unit"`
			} `json:"premises"`
			CSVHeader []string `json:"csv_header"`
			Resources []struct {
				Export []struct {
					JSONL string   `json:"jsonl"`
					CSV   []string `json:"csv"`
				} `json:"export"`
			} `json:"resources"`
		} `json:"view"`
	}
	decodeNumbers(t, pageData(t, page), &data)
	var embedded, want any
	decodeNumbers(t, data.Report, &embedded)
	decodeNumbers(t, js, &want)
	if !reflect.DeepEqual(embedded, want) {
		t.Error("html: embedded report differs from the JSON report")
	}
	// The premises panel lists as missing exactly the usage_not_declared of the report.
	var rep struct {
		Resources []struct {
			Address string `json:"address"`
		} `json:"resources"`
		Undeclared []struct {
			Address string `json:"address"`
		} `json:"usage_not_declared"`
	}
	decodeNumbers(t, js, &rep)
	var missing, undeclared []string
	for _, p := range data.View.Premises {
		if p.PerUnit != "" {
			missing = append(missing, rep.Resources[p.Resource].Address)
		}
	}
	for _, u := range rep.Undeclared {
		undeclared = append(undeclared, u.Address)
	}
	if !slices.Equal(missing, undeclared) {
		t.Errorf("html: premises missing %q, report usage_not_declared %q", missing, undeclared)
	}
	var lines strings.Builder
	for _, r := range data.View.Resources {
		for _, x := range r.Export {
			lines.WriteString(x.JSONL + "\n")
			var doc map[string]any
			decodeNumbers(t, []byte(x.JSONL), &doc)
			if len(x.CSV) != len(data.View.CSVHeader) {
				t.Fatalf("csv: %d cells, header has %d", len(x.CSV), len(data.View.CSVHeader))
			}
			for k := range doc {
				if !slices.Contains(data.View.CSVHeader, k) {
					t.Errorf("csv header misses the JSON Lines field %s", k)
				}
			}
			for i, k := range data.View.CSVHeader {
				cell := ""
				switch v := doc[k].(type) {
				case string:
					cell = v
				case json.Number:
					cell = v.String()
				case bool:
					cell = strconv.FormatBool(v)
				}
				if x.CSV[i] != cell {
					t.Errorf("csv %s = %q, json lines has %q", k, x.CSV[i], cell)
				}
			}
		}
	}
	// The export holds the cost lines only: warnings and ignored resources
	// have their own panels.
	var costLines strings.Builder
	for _, l := range strings.SplitAfter(string(jl), "\n") {
		if strings.Contains(l, `"kind":"costgate.line"`) {
			costLines.WriteString(l)
		}
	}
	if lines.String() != costLines.String() {
		t.Error("html: export lines differ from the cost lines of the JSON Lines output")
	}
}

// TestHTMLEscaping renders addresses, a provider and an instance type that
// try to close the data <script> element and inject markup.
func TestHTMLEscaping(t *testing.T) {
	const payload = "</script><img src=x onerror=alert(1)>"
	o := runCase(t, filepath.Join(root, "cases", "escape"), fixtureSnapshot(t))
	if n := bytes.Count(o.html, []byte(scriptClose)); n != 2 {
		t.Errorf("html has %d %s, want 2 (script and data)", n, scriptClose)
	}
	if bytes.Contains(o.html, []byte("<img")) {
		t.Error("html contains raw <img markup")
	}
	var data struct {
		Report struct {
			Resources []struct {
				Address string `json:"address"`
				After   struct {
					Unpriced string `json:"unpriced"`
				} `json:"after"`
			} `json:"resources"`
		} `json:"report"`
		View struct {
			Modules []struct {
				Key string `json:"key"`
			} `json:"modules"`
		} `json:"view"`
	}
	decodeNumbers(t, pageData(t, o.html), &data)
	var found, reasons int
	for _, r := range data.Report.Resources {
		if strings.Contains(r.Address, payload) || strings.Contains(r.Address, "<img src=x onerror=alert(1)>") {
			found++
		}
		if strings.Contains(r.After.Unpriced, payload) {
			reasons++
		}
	}
	if found != 3 || reasons != 2 {
		t.Errorf("payload round trip: %d addresses (want 3), %d reasons (want 2)", found, reasons)
	}
	modules := map[string]bool{}
	for _, m := range data.View.Modules {
		modules[m.Key] = true
	}
	if !modules[`module.m["a.b`+payload+`"]`] || !modules["(root)"] || len(modules) != 2 {
		t.Errorf("modules: %v", modules)
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
	v2 := runCase(t, dir, fixtureSnapshot(t))
	v1 := runCase(t, dir, filepath.Join(root, "pricing.v1.json"))
	jsV2, jsV1 := v2.js, v1.js
	if v2.code != v1.code {
		t.Fatalf("exit %d with v2, %d with v1", v2.code, v1.code)
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
		"no region prices": {"diff", "--plan", plan, "--region", "cn-north-1"},
		"two snapshots":    {"diff", "--plan", plan, "--pricing", filepath.Join(root, "pricing.golden.json"), "--pricing", filepath.Join(root, "pricing.golden.json")},
		"refresh region":   {"pricing", "refresh", "--region", "cn-north-1", "--out", filepath.Join(tmp, "x.json")},
		"regions argument": {"pricing", "regions", "extra"},
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
	if n := strings.Count(stdout.String(), "\nok ") + 1; !strings.HasPrefix(stdout.String(), "ok ") || n != 36 {
		t.Errorf("verify checked %d snapshots, want 36:\n%s", n, stdout.String())
	}
	for _, r := range []string{"us-east-1", "us-west-2", "eu-west-1", "us-gov-west-1", "ap-southeast-7"} {
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

// The workshop plan (provider in us-west-2) needs no flag: the plan names
// the region and its snapshot is embedded.
func TestEmbeddedRegionFromPlan(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"diff", "--plan", filepath.Join(root, "cases", "region-provider", "plan.json"), "--format", "json"},
		Env{Version: "test", Stdout: &stdout, Stderr: &stderr})
	if code != 0 && code != 2 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"region": "us-west-2"`) || !strings.Contains(out, `"usage_type": "USW2-PublicIPv4:InUseAddress"`) ||
		strings.Contains(out, "USE1-") || strings.Contains(out, `"code": "region_`) {
		t.Errorf("us-west-2 run does not price with USW2 keys or warns:\n%s", out)
	}
	// --region names the region of the resources the plan does not place;
	// the others stay in their own region.
	stdout.Reset()
	code = Run([]string{"diff", "--plan", filepath.Join(root, "cases", "region-provider", "plan.json"), "--region", "us-east-1", "--format", "json"},
		Env{Version: "test", Stdout: &stdout, Stderr: &stderr})
	if code != 0 && code != 2 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, `"usage_type": "USW2-PublicIPv4:InUseAddress"`) || strings.Contains(out, `"code": "region_`) {
		t.Errorf("--region us-east-1 run:\n%s", out)
	}
}

func TestPricingRegions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"pricing", "regions"}, Env{Version: "test", Stdout: &stdout, Stderr: &stderr}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 36 || lines[0] != "af-south-1\tembedded" || strings.Contains(stdout.String(), "refresh only") {
		t.Errorf("pricing regions:\n%s", stdout.String())
	}
}
