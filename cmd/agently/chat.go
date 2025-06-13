package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/service"
	"github.com/viant/fluxor"
	fluxexec "github.com/viant/fluxor/service/executor"

	elog "github.com/viant/agently/internal/log"
)

// ChatCmd handles interactive/chat queries.
type ChatCmd struct {
	Location  string `short:"l" long:"location" description:"agent definition path"`
	Query     string `short:"q" long:"query"    description:"user query"`
	ConvID    string `short:"c" long:"conv"     description:"conversation ID (optional)"`
	Policy    string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
	LLMLog    string `short:"a" long:"llm-log" description:"file to append raw LLM traffic"`
	ResetLogs bool   `long:"reset-logs" description:"truncate/clean log files before each run"  `
	Timeout   int    `short:"t" long:"timeout" description:"timeout in seconds for the agent response (0=none)" `
	ToolLog   string `long:"tool-log" description:"file to append debug logs for each tool call"`
	TaskLog   string `long:"task-log" description:"file to append per-task Fluxor executor log"`
	Log       string `long:"log" description:"unified log (LLM, TOOL, TASK)" default:"agently.log"`
}

// cliInteractionHandler satisfies service.InteractionHandler by prompting the
// user on STDIN when the assistant requests additional information.
type cliInteractionHandler struct{}

func (cliInteractionHandler) Accept(ctx context.Context, el *plan.Elicitation) ([]byte, bool, error) {
	res, err := newStdinAwaiter().AwaitElicitation(ctx, el)
	if err != nil {
		return nil, false, err
	}
	switch res.Action {
	case plan.ElicitResultActionDecline:
		return nil, false, nil
	case plan.ElicitResultActionAccept:
		if len(res.Payload) == 0 {
			return nil, false, nil
		}
		data, err := json.Marshal(res.Payload)
		if err != nil {
			return nil, false, err
		}
		return data, true, nil
	default:
		return nil, false, nil
	}
}

func (c *ChatCmd) Execute(_ []string) error {
	// Fallbacks -------------------------------------------------------
	if c.Location == "" {
		c.Location = "chat" // default agent shipped with embedded config
	}

	// Reset logs if requested ------------------------------------------------------
	if c.ResetLogs {
		if c.LLMLog != "" {
			_ = os.Remove(c.LLMLog)
		}
		if c.ToolLog != "" {
			_ = os.Remove(c.ToolLog)
		}
		if c.TaskLog != "" {
			_ = os.Remove(c.TaskLog)
		}
		if c.Log != "" {
			_ = os.Remove(c.Log)
		}
	}

	// Prepare optional log writers ------------------------------------------------
	logOpenFlags := os.O_CREATE | os.O_WRONLY
	if !c.ResetLogs {
		logOpenFlags |= os.O_APPEND
	} else {
		logOpenFlags |= os.O_TRUNC
	}

	// Uber log writer ------------------------------------------------------
	var log io.Writer
	if c.Log == "" {
		c.Log = "agently.log"
	}
	if w, err := os.OpenFile(c.Log, logOpenFlags, 0644); err == nil {
		log = w
	} else {
		fmt.Printf("warning: unable to open uber log file %s: %v\n", c.Log, err)
	}

	if log != nil {
		elog.FileSink(log,
			elog.LLMInput, elog.LLMOutput,
			elog.TaskInput, elog.TaskOutput,
			elog.TaskWhen,
		)
		registerExecOption(executor.WithWorkflowOptions(
			fluxor.WithExecutorOptions(
				fluxexec.WithListener(newJSONTaskListener()),
			),
		))
	}

	{
		var llmWriters []io.Writer
		if c.LLMLog != "" {
			if w, err := os.OpenFile(c.LLMLog, logOpenFlags, 0644); err == nil {
				llmWriters = append(llmWriters, w)
			} else {
				fmt.Printf("warning: unable to open llm log file %s: %v\n", c.LLMLog, err)
			}
		}
		if len(llmWriters) > 0 {
			registerExecOption(executor.WithLLMLogger(io.MultiWriter(llmWriters...)))
		}
	}

	{
		var toolWriters []io.Writer
		if c.ToolLog != "" {
			if w, err := os.OpenFile(c.ToolLog, logOpenFlags, 0644); err == nil {
				toolWriters = append(toolWriters, w)
			} else {
				fmt.Printf("warning: unable to open tool log file %s: %v\n", c.ToolLog, err)
			}
		}
		if len(toolWriters) > 0 {
			registerExecOption(executor.WithToolDebugLogger(io.MultiWriter(toolWriters...)))
		}
	}

	{
		var taskWriters []io.Writer
		if c.TaskLog != "" {
			if w, err := os.OpenFile(c.TaskLog, logOpenFlags, 0644); err == nil {
				taskWriters = append(taskWriters, w)
			} else {
				fmt.Printf("warning: unable to open task log file %s: %v\n", c.TaskLog, err)
			}
		}
		if len(taskWriters) > 0 {
			listener := newJSONTaskListener(taskWriters...)
			registerExecOption(executor.WithWorkflowOptions(
				fluxor.WithExecutorOptions(
					fluxexec.WithListener(listener),
				),
			))
		}
	}

	// Build executor and service --------------------------------------------
	svcExec := executorSingleton()

	ctxBase, cancel := context.WithCancel(context.Background())
	defer cancel()

	fluxPol := buildFluxorPolicy(c.Policy)
	toolPol := &tool.Policy{Mode: fluxPol.Mode, Ask: stdinAsk}

	stopApprove := startApprovalLoop(ctxBase, svcExec, fluxPol)
	defer stopApprove()

	serviceOpts := service.Options{Interaction: cliInteractionHandler{}}
	svc := service.New(svcExec, serviceOpts)

	convID := c.ConvID

	callChat := func(userQuery string) error {
		ctx := tool.WithPolicy(ctxBase, toolPol)
		ctx = withFluxorPolicy(ctx, fluxPol)
		resp, err := svc.Chat(ctx, service.ChatRequest{
			ConversationID: convID,
			AgentPath:      c.Location,
			Query:          userQuery,
			Timeout:        time.Duration(c.Timeout) * time.Second,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Println("[no response] - timeout")
				return nil
			}
			return err
		}
		convID = resp.ConversationID
		if strings.TrimSpace(resp.Content) == "" {
			fmt.Println("[no response] - no content")
		} else {
			fmt.Println(resp.Content)
		}
		return nil
	}

	// Single-turn when -q provided.
	if c.Query != "" {
		if err := callChat(c.Query); err != nil {
			return err
		}
		fmt.Printf("[conversation-id] %s\n", convID)
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" || line == "exit" || line == "quit" {
			fmt.Printf("[conversation-id] %s\n", convID)
			break
		}
		if err := callChat(line); err != nil {
			return err
		}
	}
	return nil
}
