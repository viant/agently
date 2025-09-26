package main

import (
	"os"

	_ "github.com/viant/agently/pkg/dependency"
	"github.com/viant/datly/cmd"
	"github.com/viant/datly/view/extension"
)

func main() {
	extension.InitRegistry()
	err := cmd.RunApp("version", os.Args[1:])
	if err != nil {
		panic(err)
	}
}
