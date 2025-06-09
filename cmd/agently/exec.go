package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/fluxor-mcp/mcp/tool"
	"io"
	"os"
	"time"
)

// ExecCmd executes a registered tool via the Agently executor. It mirrors the
// behaviour of fluxor-mcp's `exec` command so that users switching between the
// two CLIs get a consistent experience.
type ExecCmd struct {
	Name       string `short:"n" long:"name" positional-arg-name:"tool" description:"Tool name (service_method)" required:"yes"`
	Inline     string `short:"i" long:"input" description:"Inline JSON arguments (object)"`
	File       string `short:"f" long:"file" description:"Path to JSON file with arguments (use - for stdin)"`
	TimeoutSec int    `long:"timeout" description:"Seconds to wait for completion" default:"120"`
	JSON       bool   `long:"json" description:"Print result as JSON"`
}

func (c *ExecCmd) Execute(_ []string) error {
	if c.Inline != "" && c.File != "" {
		return fmt.Errorf("-i/--input and -f/--file are mutually exclusive")
	}

	svc := executorSingleton()
	time.Sleep(100 * time.Millisecond)
	// ------------------------------------------------------------------
	// Build arguments map
	// ------------------------------------------------------------------
	var args map[string]interface{}

	switch {
	case c.Inline != "":
		if err := json.Unmarshal([]byte(c.Inline), &args); err != nil {
			return fmt.Errorf("invalid inline JSON: %w", err)
		}
	case c.File != "":
		var rdr io.Reader
		if c.File == "-" {
			rdr = os.Stdin
		} else {
			f, err := os.Open(c.File)
			if err != nil {
				return fmt.Errorf("open input file: %w", err)
			}
			defer f.Close()
			rdr = f
		}
		data, err := io.ReadAll(rdr)
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return fmt.Errorf("decode JSON: %w", err)
		}
	default:
		// no arguments supplied – keep args nil
	}

	ctx := context.Background()
	timeout := time.Duration(c.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	canonical := tool.Canonical(c.Name)
	out, err := svc.ExecuteTool(ctx, canonical, args, timeout)
	if err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Print result
	// ------------------------------------------------------------------
	if c.JSON {
		bytes, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(bytes))
	} else {
		switch v := out.(type) {
		case string:
			fmt.Println(v)
		case []byte:
			fmt.Println(string(v))
		default:
			bytes, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(bytes))
		}
	}
	return nil
}
