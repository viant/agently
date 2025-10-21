package main

import (
	"os"

	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	"github.com/viant/agently/cmd/agently"
)

// Version is populated by build ldflags in CI/release builds.
// Default value is "dev" for local builds.
var Version = "dev"

func main() {

	os.Setenv("AGENTLY_ROOT", "/Users/awitas/go/src/github.com/viant/agently/ag")
	os.Args = []string{"", "serve"}

	// Expose version to the CLI layer so `-v/--version` can print it.
	agently.SetVersion(Version)
	agently.RunWithCommands(os.Args[1:])
}
