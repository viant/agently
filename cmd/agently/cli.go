package agently

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
	"github.com/viant/agently/internal/workspace"
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

	// Handle version early to avoid command requirement error from parser
	if hasVersionFlag(args) {
		fmt.Println(Version())
		os.Exit(0)
	}

	// Print startup workspace to make it clear which workspace is in use.
	// We show both the raw env value and the resolved absolute path.
	envWS := strings.TrimSpace(os.Getenv("AGENTLY_WORKSPACE"))
	// Calling Root also ensures the workspace exists.
	resolvedWS := workspace.Root()
	if envWS != "" {
		log.Printf("Starting Agently workspace with ${env.AGENTLY_WORKSPACE}:  %s", resolvedWS)
	} else {
		log.Printf("Starting Agently workspace with default workspace:  %s, ${env.AGENTLY_WORKSPACE} not set", resolvedWS)

	}
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

// hasVersionFlag returns true if args contain a global version flag.
func hasVersionFlag(args []string) bool {
	for _, a := range args {
		if a == "-v" || a == "--version" {
			return true
		}
	}
	return false
}

// RunWithCommands is kept for symmetry with scy CLI.
func RunWithCommands(args []string) {
	Run(args)
}
