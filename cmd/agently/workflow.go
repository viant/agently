package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/service"

	elog "github.com/viant/agently/internal/log"
)

// WorkflowCmd executes a Fluxor workflow graph directly.
type WorkflowCmd struct {
	Location string `short:"l" long:"location" description:"workflow YAML/JSON location"`
	TaskID   string `short:"t" long:"task" description:"optional task ID to run (single task)"`

	Inline string `short:"i" long:"input" description:"inline JSON input object"`
	File   string `short:"f" long:"file"  description:"path to JSON file with input (use - for STDIN)"`

	Timeout int    `long:"timeout" description:"timeout in seconds (0 = no timeout)"`
	Policy  string `long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`

	Log string `long:"log" description:"append raw workflow output to this file"`
}

func (w *WorkflowCmd) readInput() (interface{}, error) {
	if strings.TrimSpace(w.Inline) == "" && w.File == "" {
		return nil, nil
	}

	var reader io.Reader
	switch {
	case w.Inline != "":
		reader = strings.NewReader(w.Inline)
	case w.File == "-":
		reader = os.Stdin
	default:
		f, err := os.Open(w.File)
		if err != nil {
			return nil, fmt.Errorf("open input file: %w", err)
		}
		defer f.Close()
		reader = f
	}

	var data interface{}
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode JSON input: %w", err)
	}
	return data, nil
}

func (w *WorkflowCmd) Execute(_ []string) error {
	if w.Location == "" {
		return fmt.Errorf("workflow location is required (-l)")
	}

	input, err := w.readInput()
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	// Log file (optional)
	if w.Log != "" {
		lf, err := os.OpenFile(w.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			elog.FileSink(lf, elog.TaskInput, elog.TaskOutput, elog.TaskWhen)
		}
	}

	execSvc := executorSingleton()

	toolPol := &tool.Policy{Mode: buildFluxorPolicy(w.Policy).Mode, Ask: stdinAsk}

	svc := service.New(execSvc, service.Options{})

	ctx := tool.WithPolicy(context.Background(), toolPol)

	var timeout time.Duration
	if w.Timeout > 0 {
		timeout = time.Duration(w.Timeout) * time.Second
	}

	resp, err := svc.ExecuteWorkflow(ctx, service.WorkflowRequest{
		Location: w.Location,
		TaskID:   w.TaskID,
		Input:    input,
		Timeout:  timeout,
	})

	if err != nil {
		return err
	}

	pretty, _ := json.MarshalIndent(resp.Output, "", "  ")
	fmt.Println(string(pretty))

	return nil
}
