package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/viant/agently/genai/executor"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/service"
	"github.com/viant/fluxor"
	fluxexec "github.com/viant/fluxor/service/executor"

	elog "github.com/viant/agently/internal/log"
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
		q.Location = r.Location
	}

	// Unified log writer (uber)
	uberPath := r.UberLog
	if uberPath == "" {
		uberPath = "agently.log"
	}
	log, _ := os.OpenFile(uberPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)

	if log != nil {
		elog.FileSink(log,
			elog.LLMInput, elog.LLMOutput,
			elog.TaskInput, elog.TaskOutput,
		)
		registerExecOption(executor.WithWorkflowOptions(
			fluxor.WithExecutorOptions(
				fluxexec.WithListener(newJSONTaskListener()),
			),
		))
	}

	// LLM log writers --------------------------------------------------
	{
		var writers []io.Writer
		if r.LLMLog != "" {
			if w, err := os.OpenFile(r.LLMLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				writers = append(writers, w)
			} else {
				fmt.Printf("warning: unable to open llm log file %s: %v\n", r.LLMLog, err)
			}
		}
		if len(writers) > 0 {
			registerExecOption(executor.WithLLMLogger(io.MultiWriter(writers...)))
		}
	}

	// Tool log writers -------------------------------------------------
	{
		var writers []io.Writer
		if r.ToolLog != "" {
			if w, err := os.OpenFile(r.ToolLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				writers = append(writers, w)
			} else {
				fmt.Printf("warning: unable to open tool log file %s: %v\n", r.ToolLog, err)
			}
		}
		if len(writers) > 0 {
			registerExecOption(executor.WithToolDebugLogger(io.MultiWriter(writers...)))
		}
	}

	// Task log listener -----------------------------------------------
	{
		var writers []io.Writer
		if r.TaskLog != "" {
			if w, err := os.OpenFile(r.TaskLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				writers = append(writers, w)
			} else {
				fmt.Printf("warning: unable to open task log file %s: %v\n", r.TaskLog, err)
			}
		}
		if len(writers) > 0 {
			listener := newJSONTaskListener(writers...)
			registerExecOption(executor.WithWorkflowOptions(
				fluxor.WithExecutorOptions(
					fluxexec.WithListener(listener),
				),
			))
		}
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
