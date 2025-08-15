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
	//
	os.Args = []string{"", "chat"}
	os.Args = []string{"", "chat", "--reset-logs", "-q='hi,what day do we have today ?"}
	//os.Args = []string{"", "serve"}
	agently.RunWithCommands(os.Args[1:])
}
