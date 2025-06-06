package main

import (
	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	cli "github.com/viant/agently/cmd/agently"
	"os"
)

func main() {
	cli.RunWithCommands(os.Args[1:])
}
