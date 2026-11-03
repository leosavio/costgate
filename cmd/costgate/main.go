// Command costgate projects the monthly AWS cost of a Terraform/OpenTofu
// plan and gates pull requests on it.
package main

import (
	"os"

	"github.com/leosavio/costgate/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		Version: version,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}))
}
