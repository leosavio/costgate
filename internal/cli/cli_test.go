package cli

import (
	"bytes"
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

func runCase(t *testing.T, dir, snapshot string) (md, js []byte, code int) {
	t.Helper()
	args := []string{"diff", "--plan", filepath.Join(dir, "plan.json"), "--pricing", snapshot}
	if _, err := os.Stat(filepath.Join(dir, "usage.json")); err == nil {
		args = append(args, "--usage", filepath.Join(dir, "usage.json"))
	}
	if extra, err := os.ReadFile(filepath.Join(dir, "args.txt")); err == nil {
		args = append(args, strings.Fields(string(extra))...)
	}
	jsonPath := filepath.Join(t.TempDir(), "report.json")
	args = append(args, "--json-out", jsonPath)
	var stdout, stderr bytes.Buffer
	code = Run(args, Env{Version: "test", Stdout: &stdout, Stderr: &stderr})
	if stderr.Len() > 0 {
		t.Fatalf("stderr: %s", stderr.String())
	}
	js, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	return stdout.Bytes(), js, code
}

func TestGoldenCases(t *testing.T) {
	snapshot := fixtureSnapshot(t)
	dirs, err := filepath.Glob(filepath.Join(root, "cases", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases: %v", err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			md, js, code := runCase(t, dir, snapshot)
			golden(t, filepath.Join(dir, "want.md"), md)
			golden(t, filepath.Join(dir, "want.json"), js)
			golden(t, filepath.Join(dir, "want_exit"), []byte(strconv.Itoa(code)+"\n"))

			// Determinism: a second run produces the same bytes.
			md2, js2, code2 := runCase(t, dir, snapshot)
			if !bytes.Equal(md, md2) || !bytes.Equal(js, js2) || code != code2 {
				t.Error("second run differs from the first")
			}
		})
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
