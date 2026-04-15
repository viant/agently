package agently

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/jessevdk/go-flags"
)

// Run parses flags and executes the selected command.
func Run(args []string) {
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

	parser := flags.NewParser(opts, flags.HelpFlag|flags.PassDoubleDash)
	if _, err := parser.ParseArgs(args); err != nil {
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("%v", err)
	}

	// Global version flag: print and exit successfully.
	if opts.Version {
		fmt.Println(Version())
		os.Exit(0)
	}
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
