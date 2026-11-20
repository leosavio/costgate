// Package pricing embeds the versioned price snapshots, so the binary prices
// a plan offline. Regenerate with `costgate pricing refresh`.
package pricing

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.json
var files embed.FS

// Regions lists the regions with an embedded snapshot, sorted.
func Regions() []string {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		panic(err) // embedded directory: cannot fail
	}
	var out []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".json"); ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Has reports whether region has an embedded snapshot.
func Has(region string) bool {
	_, err := fs.Stat(files, region+".json")
	return err == nil
}

// Snapshot returns the embedded snapshot of region.
func Snapshot(region string) ([]byte, error) {
	b, err := files.ReadFile(region + ".json")
	if err != nil {
		return nil, fmt.Errorf("no embedded price snapshot for region %s (embedded: %s); run 'costgate pricing refresh --region %s --out %s.json' and pass --pricing %s.json",
			region, strings.Join(Regions(), ", "), region, region, region)
	}
	return b, nil
}
