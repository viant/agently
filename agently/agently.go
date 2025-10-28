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
	//os.Setenv("AGENTLY_DB_DRIVER", "mysql")
	//os.Setenv("AGENTLY_DB_DSN", "root:dev@tcp(127.0.0.1:3307)/forecaster?parseTime=true")
	os.Setenv("AGENTLY_ROOT", "/Users/awitas/go/src/github.vianttech.com/viant/mcp_agently/runtime/polaris")
	os.Args = []string{"", "serve", "-a", ":8088"}

	// Expose version to the CLI layer so `-v/--version` can print it.
	agently.SetVersion(Version)
	agently.RunWithCommands(os.Args[1:])
}
