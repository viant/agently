package main

import (
	"os"

	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	"github.com/viant/agently"
	cagently "github.com/viant/agently/cmd/agently"
)

// Version is populated by build ldflags in CI/release builds.
// Default value is "dev" for local builds.
var Version = agently.Version

func main() {

	// Expose version to the CLI layer so `-v/--version` can print it.
	cagently.SetVersion(Version)
	cagently.RunWithCommands(os.Args[1:])
}
