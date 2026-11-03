// Package pricing embeds the versioned price snapshots, so the binary prices
// a plan offline. Regenerate with `costgate pricing refresh`.
package pricing

import (
	"embed"
	"fmt"
)

//go:embed *.json
var files embed.FS

// Snapshot returns the embedded snapshot of region.
func Snapshot(region string) ([]byte, error) {
	b, err := files.ReadFile(region + ".json")
	if err != nil {
		return nil, fmt.Errorf("no embedded price snapshot for region %s; pass --pricing", region)
	}
	return b, nil
}
