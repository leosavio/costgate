// Package pricing embeds the versioned price snapshots of every region the
// public Price List serves, gzip-compressed, so the binary prices a plan
// offline in any region. Regenerate with `costgate pricing refresh`.
package pricing

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.json.gz
var files embed.FS

const suffix = ".json.gz"

// Regions lists the regions with an embedded snapshot, sorted.
func Regions() []string {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		panic(err) // embedded directory: cannot fail
	}
	var out []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), suffix); ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Has reports whether region has an embedded snapshot.
func Has(region string) bool {
	_, err := fs.Stat(files, region+suffix)
	return err == nil
}

// Snapshot returns the embedded snapshot of region, gzip-compressed
// (catalog.Load reads it as is).
func Snapshot(region string) ([]byte, error) {
	b, err := files.ReadFile(region + suffix)
	if err != nil {
		return nil, fmt.Errorf("no embedded price snapshot for region %s; run 'costgate pricing refresh --region %s --out %s.json.gz' and pass --pricing %s.json.gz",
			region, region, region, region)
	}
	return b, nil
}
