package main

import (
	"os"

	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	"github.com/viant/agently/cmd/agently"
)

func main() {

	//os.Setenv("AGENTLY_ROOT", "/Users/awitas/go/src/github.com/viant/agently/ag")
	//os.Args = []string{"", "chat", "-a=coder", "-q='my local runner sometimes is unable detect that command has complete and waits till timeout, here is test failint due to this issue: /Users/awitas/go/src/github.com/viant/gosh/runner/local/runner_test.go '"}
	//os.Args = []string{"", "serve"}
	agently.RunWithCommands(os.Args[1:])
}
