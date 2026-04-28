package main

import (
	"os"

	"github.com/google/gops/agent"
	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/aws/secretmanager"
	_ "github.com/viant/afsc/aws/ssm"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gcp/secretmanager"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	"github.com/viant/agently"
	cagently "github.com/viant/agently/cmd/agently"
	_ "github.com/viant/bigquery"
)

var Version = agently.Version

func main() {
	_ = agent.Listen(agent.Options{})
	cagently.SetVersion(Version)
	cagently.RunWithCommands(os.Args[1:])
}
