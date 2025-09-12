package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/viant/agently/cmd/service"
	agentpkg "github.com/viant/agently/genai/service/agent"
	"github.com/viant/agently/genai/tool"
)

// RunCmd executes full agentic workflow from JSON payload.
type RunCmd struct {
	Location  string `short:"l" long:"location" description:"agent definition path"`
	InputFile string `short:"i" long:"input"    description:"JSON file with QueryInput (stdin if empty)"`
	Policy    string `long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
	LLMLog    string `long:"llm-log" description:"file to append raw LLM traffic"`
	ToolLog   string `long:"tool-log" description:"file to append debug logs for each tool call"`
	TaskLog   string `long:"task-log" description:"file to append per-task Fluxor executor log"`
	UberLog   string `long:"log" description:"unified log (LLM, TOOL, TASK)" default:"agently.log"`
}

func (r *RunCmd) Execute(_ []string) error {
	var reader io.Reader = os.Stdin
	if r.InputFile != "" {
		f, err := os.Open(r.InputFile)
		if err != nil {
			return fmt.Errorf("open input: %w", err)
		}
		defer f.Close()
		reader = f
	}

	var q agentpkg.QueryInput
	if err := json.NewDecoder(reader).Decode(&q); err != nil {
		return fmt.Errorf("decode input: %w", err)
	}
	if r.Location != "" {
		q.AgentName = r.Location
	}

	svc := executorSingleton()
	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fluxPol := buildFluxorPolicy(r.Policy)
	toolPol := &tool.Policy{Mode: fluxPol.Mode, Ask: stdinAsk}
	stopApprove := startApprovalLoop(baseCtx, svc, fluxPol)
	defer stopApprove()

	ctx := tool.WithPolicy(baseCtx, toolPol)
	ctx = withFluxorPolicy(ctx, fluxPol)

	// Build service wrapper (no interaction handler since run is one-shot)
	agentlySvc := service.New(svc, service.Options{})

	resp, err := agentlySvc.Run(ctx, service.RunRequest{
		Input:   &q,
		Timeout: 0,
		Policy:  toolPol,
	})
	if err != nil {
		return err
	}

	bytes, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(bytes))
	return nil
}
