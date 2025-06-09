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
	os.Args = []string{"", "list-tools", "-n", "printer-print"}
	agently.RunWithCommands(os.Args[1:])
}
