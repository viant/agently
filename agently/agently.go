package main

import (
	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	"github.com/viant/agently/cmd/agently"
	"os"
)

func main() {

	//os.Setenv("AGENTLY_ROOT", "/Users/awitas/go/src/github.com/viant/agently/ag")

	//os.Args = []string{"", "chat"}
	//os.Args = []string{"", "chat", "--reset-logs", "-q='hi,what country  is my favourite city ?"}
	//os.Args = []string{"", "serve"}
	agently.RunWithCommands(os.Args[1:])
}
