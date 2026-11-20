package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io/fs"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureMirror copies the reduced Bulk API mirror of testdata/offers into a
// temporary directory and adds an eu-west-1 file next to every us-east-1
// regional offer file: region code, location and usage type prefix ("EU-",
// the odd one of the Price List) rewritten, and every price times 1.25, so a
// test can tell which region priced a line.
func fixtureMirror(t *testing.T) string {
	t.Helper()
	src := filepath.Join(root, "offers")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := filepath.Glob(filepath.Join(dst, "offers", "v1.0", "aws", "*", "current", "region_index.json"))
	if err != nil || len(indexes) == 0 {
		t.Fatalf("region indexes: %v", err)
	}
	for _, idx := range indexes {
		var ri struct {
			Regions map[string]struct {
				RegionCode        string `json:"regionCode"`
				CurrentVersionURL string `json:"currentVersionUrl"`
			} `json:"regions"`
		}
		data, err := os.ReadFile(idx)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &ri); err != nil {
			t.Fatal(err)
		}
		east, ok := ri.Regions["us-east-1"]
		if !ok {
			t.Fatalf("%s: no us-east-1", idx)
		}
		west := east
		west.RegionCode = "eu-west-1"
		west.CurrentVersionURL = strings.Replace(east.CurrentVersionURL, "/us-east-1/", "/eu-west-1/", 1)
		ri.Regions["eu-west-1"] = west
		csvOf := func(u string) string {
			return filepath.Join(dst, filepath.FromSlash(strings.TrimSuffix(u, ".json")+".csv"))
		}
		in, err := os.ReadFile(csvOf(east.CurrentVersionURL))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(csvOf(west.CurrentVersionURL)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(csvOf(west.CurrentVersionURL), asEUWest1(t, in), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := json.MarshalIndent(ri, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(idx, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

// asEUWest1 rewrites a us-east-1 regional offer file as eu-west-1.
func asEUWest1(t *testing.T, in []byte) []byte {
	t.Helper()
	r := csv.NewReader(bytes.NewReader(in))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil || len(recs) < 6 {
		t.Fatalf("offer file: %d records, %v", len(recs), err)
	}
	col := map[string]int{}
	for i, h := range recs[5] {
		col[h] = i
	}
	quarter := big.NewRat(5, 4)
	rc, hasRC := col["Region Code"]
	for _, rec := range recs[6:] {
		if hasRC {
			if rec[rc] != "us-east-1" {
				continue
			}
			rec[rc] = "eu-west-1"
		}
		for i := range rec {
			rec[i] = strings.ReplaceAll(rec[i], "US East (N. Virginia)", "EU (Ireland)")
		}
		ut := rec[col["usageType"]]
		switch {
		case strings.HasPrefix(ut, "USE1-"):
			ut = "EU-" + strings.TrimPrefix(ut, "USE1-")
		case strings.HasPrefix(ut, "us-east-1-"):
			ut = "eu-west-1-" + strings.TrimPrefix(ut, "us-east-1-")
		default:
			ut = "EU-" + ut
		}
		rec[col["usageType"]] = ut
		if p, ok := new(big.Rat).SetString(rec[col["PricePerUnit"]]); ok {
			rec[col["PricePerUnit"]] = new(big.Rat).Mul(p, quarter).FloatString(10)
		}
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(recs); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixtureSnapshots refreshes the us-east-1 and eu-west-1 snapshots from
// fixtureMirror and returns their paths.
func fixtureSnapshots(t *testing.T) (east, west string) {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir(fixtureMirror(t))))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	for _, region := range []string{"us-east-1", "eu-west-1"} {
		out := filepath.Join(dir, region+".json")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pricing", "refresh", "--region", region, "--strict", "--base-url", srv.URL, "--out", out},
			Env{Version: "test", Stdout: &stdout, Stderr: &stderr, HTTPClient: srv.Client()})
		if code != 0 {
			t.Fatalf("refresh %s exit %d: %s", region, code, stderr.String())
		}
	}
	return filepath.Join(dir, "us-east-1.json"), filepath.Join(dir, "eu-west-1.json")
}

// fixtureEnv is the Env of the golden cases: the built-in snapshots are the
// two fixture snapshots, so the reports do not change with the weekly price
// refresh of the real ones.
func fixtureEnv(t *testing.T, east, west string) Env {
	t.Helper()
	files := map[string][]byte{}
	for region, path := range map[string]string{"us-east-1": east, "eu-west-1": west} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[region] = data
	}
	return Env{Version: "test", Embedded: func(region string) ([]byte, bool) {
		data, ok := files[region]
		return data, ok
	}}
}
