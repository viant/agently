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
	//
	os.Setenv("AGENTLY_DB_DRIVER", "mysql")
	os.Setenv("AGENTLY_DB_DSN", "root:dev@tcp(127.0.0.1:3307)/agently?parseTime=true")
	os.Setenv("AGENTLY_V1_DOMAIN", "1")
	os.Setenv("AGENTLY_DOMAIN_MODE", "full")
	os.Setenv("AGENTLY_ROOT", "/Users/awitas/go/src/github.com/viant/agently/ag")
	os.Args = []string{"", "serve"}

	//os.Args = []string{"", "chat", "-q=now plan trip for next week to my favorite city?", "-c=501451ae-3e0b-4d47-89e6-4c32e42eda77"}

	//os.Args = []string{"", "serve"}
	//os.Args = []string{"", "serve"}
	agently.RunWithCommands(os.Args[1:])

	//os.Args = []string{"", "chat",
	//	"-q=how many days till end of the year", "-c=501451ae-3e0b-4d47-89e6-4c32e42eda74"}
	//agently.RunWithCommands(os.Args[1:])
}
