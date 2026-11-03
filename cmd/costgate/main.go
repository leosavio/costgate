// Command costgate projects the monthly AWS cost of a Terraform/OpenTofu
// plan and gates pull requests on it.
package main

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/leosavio/costgate/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=<version>".
var version = "dev"

// resolveVersion falls back to the module version recorded by `go install
// module@vX.Y.Z`, which does not apply -ldflags.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return version
}

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		Version: resolveVersion(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}))
}
