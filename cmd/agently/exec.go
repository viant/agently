package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	mcpclienthandler "github.com/viant/agently/adapter/mcp"
	mcpmgr "github.com/viant/agently/adapter/mcp/manager"
	mcprouter "github.com/viant/agently/adapter/mcp/router"
	convfactory "github.com/viant/agently/client/conversation/factory"
	"github.com/viant/agently/cmd/service"
	elicitation "github.com/viant/agently/genai/elicitation"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/fluxor-mcp/mcp/tool"
	protoclient "github.com/viant/mcp-protocol/client"
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

	// Ensure per-conversation MCP manager is available for this ad-hoc exec path.
	// Use a stdin awaiter so elicitation can be completed interactively.
	prov := mcpmgr.NewRepoProvider()
	r := mcprouter.New()
	// Build conversation client from env for persistence
	convClient, err := convfactory.NewFromEnv(context.Background())
	if err != nil {
		return err
	}
	mgr := mcpmgr.New(prov, mcpmgr.WithHandlerFactory(func() protoclient.Handler {
		// Elicitation service for tool elicitations, share router; attach stdin awaiter
		el := elicitation.New(convClient, nil, r, newStdinAwaiter)
		return mcpclienthandler.NewClient(el, convClient, nil)
	}))
	registerExecOption(executor.WithMCPManager(mgr))

	execSvc := executorSingleton()
	svc := service.New(execSvc, service.Options{})
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
		timeout = 15 * time.Minute
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create a synthetic conversation id so per-conversation MCP clients and
	// elicitation routing work consistently for ad-hoc exec as well.
	convID := uuid.New().String()
	turn := memory.TurnMeta{
		ConversationID:  convID,
		ParentMessageID: uuid.New().String(),
		TurnID:          uuid.New().String(),
	}
	ctx = memory.WithConversationID(ctx, convID)
	ctx = memory.WithTurnMeta(ctx, turn)
	canonical := tool.Canonical(c.Name)
	resp, err := svc.ExecuteTool(ctx, service.ToolRequest{
		Name:    canonical,
		Args:    args,
		Timeout: timeout,
	})
	if err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Print result
	// ------------------------------------------------------------------
	if c.JSON {
		bytes, _ := json.MarshalIndent(resp.Result, "", "  ")
		fmt.Println(string(bytes))
	} else {
		switch v := resp.Result.(type) {
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
