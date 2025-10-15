package agently

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
)

// Run parses flags and executes the selected command.
func Run(args []string) {
	cfgPath := extractConfigPath(args)
	setConfigPath(cfgPath)

	opts := &Options{}
	var first string
	if len(args) > 0 {
		first = args[0]
	}
	opts.Init(first)

	parser := flags.NewParser(opts, flags.HelpFlag|flags.PassDoubleDash)
	if _, err := parser.ParseArgs(args); err != nil {
		// flags already prints user-friendly message; we only exit with code 1
		log.Fatalf("%v", err)
	}

	// Global version flag: print and exit successfully.
	if opts.Version {
		fmt.Println(Version())
		os.Exit(0)
	}
}

// extractConfigPath scans raw args for -f/--config before full parsing so that
// the singleton can be initialised by sub-command Execute.
func extractConfigPath(args []string) string {
	for i, a := range args {
		switch a {
		case "-f", "--config":
			if i+1 < len(args) {
				return args[i+1]
			}
		default:
			if strings.HasPrefix(a, "--config=") {
				return strings.TrimPrefix(a, "--config=")
			}
		}
	}
	return ""
}

// RunWithCommands is kept for symmetry with scy CLI.
func RunWithCommands(args []string) {
	Run(args)
}
