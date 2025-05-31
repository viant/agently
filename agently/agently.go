package main

import (
	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	"github.com/viant/agently/cmd/agently"
	"os"
	"path"
)

func main() {
	baseDir := "/Users/awitas/go/src/github.com/viant/agently"

	os.Args = []string{"", "chat", "-a" + path.Join(baseDir, "llm.log"), "-w" + path.Join(baseDir, "workflow.log")}
	agently.RunWithCommands(os.Args[1:])

}
