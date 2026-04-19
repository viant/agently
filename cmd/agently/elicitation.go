package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	coreplan "github.com/viant/agently-core/protocol/agent/plan"
)

// readPromptLine reads one trimmed line from the reader, honouring ctx. If
// ctx fires before the user types, the call returns with cancelled=true and
// the ctx error — the blocked ReadString goroutine is abandoned, which is safe
// because the caller's command is about to unwind. If the underlying stream
// closes (io.EOF) we treat it as the user pressing Ctrl-D. For other IO
// errors we return them so the caller can surface the failure instead of
// looping on an empty prompt.
func readPromptLine(ctx context.Context, reader *bufio.Reader) (line string, cancelled bool, err error) {
	type result struct {
		raw string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, rerr := reader.ReadString('\n')
		ch <- result{raw: raw, err: rerr}
	}()
	select {
	case <-ctx.Done():
		return "", true, ctx.Err()
	case r := <-ch:
		line = strings.TrimSpace(r.raw)
		switch {
		case r.err == nil:
			return line, false, nil
		case errors.Is(r.err, io.EOF):
			return line, true, nil
		default:
			return line, true, r.err
		}
	}
}

func awaitCoreElicitation(ctx context.Context, req *coreplan.Elicitation) (*coreplan.ElicitResult, error) {
	if req == nil || req.IsEmpty() {
		return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionAccept}, nil
	}
	return awaitFormElicitation(ctx, os.Stdout, os.Stdin, req)
}

func awaitFormElicitation(ctx context.Context, w io.Writer, r io.Reader, req *coreplan.Elicitation) (*coreplan.ElicitResult, error) {
	reader := bufio.NewReader(r)

	fmt.Fprintf(w, "\n--- Elicitation ---\n%s\n", req.Message)
	if meta := parseToolApprovalMeta(req); meta != nil {
		return awaitToolApprovalElicitation(ctx, w, reader, req, meta)
	}

	payload := map[string]any{}
	for name, prop := range req.RequestedSchema.Properties {
		desc := name
		if pm, ok := prop.(map[string]any); ok {
			if d, ok := pm["description"].(string); ok && d != "" {
				desc = d
			}
		}
		fmt.Fprintf(w, "  %s (or 'next' to skip, 'cancel' to cancel): ", desc)
		value, cancelled, err := readPromptLine(ctx, reader)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				fmt.Fprintln(w, "")
				return nil, err
			}
			log.Printf("[elicitation] read input failed: %v", err)
		}
		if cancelled {
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
		}
		switch strings.ToLower(value) {
		case "cancel":
			return &coreplan.ElicitResult{
				Action: coreplan.ElicitResultActionDecline,
				Reason: "cancelled",
			}, nil
		case "next":
			continue
		default:
			payload[name] = value
		}
	}

	for {
		fmt.Fprint(w, "Submit? [a]ccept, [c]ancel (default: a): ")
		line, cancelled, err := readPromptLine(ctx, reader)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				fmt.Fprintln(w, "")
				return nil, err
			}
			log.Printf("[elicitation] read input failed: %v", err)
		}
		if cancelled {
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
		}
		sel := strings.ToLower(line)
		if sel == "" || sel == "a" || sel == "accept" {
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionAccept, Payload: payload}, nil
		}
		if sel == "c" || sel == "cancel" {
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
		}
		fmt.Fprintln(w, "Invalid choice.")
	}
}

type cliApprovalMeta struct {
	Type        string               `json:"type"`
	Title       string               `json:"title"`
	ToolName    string               `json:"toolName"`
	Message     string               `json:"message"`
	AcceptLabel string               `json:"acceptLabel"`
	RejectLabel string               `json:"rejectLabel"`
	CancelLabel string               `json:"cancelLabel"`
	Editors     []*cliApprovalEditor `json:"editors"`
}

type cliApprovalEditor struct {
	Name        string               `json:"name"`
	Kind        string               `json:"kind"`
	Label       string               `json:"label"`
	Description string               `json:"description"`
	Options     []*cliApprovalOption `json:"options"`
}

type cliApprovalOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Selected    bool   `json:"selected"`
}

func parseToolApprovalMeta(req *coreplan.Elicitation) *cliApprovalMeta {
	if req == nil {
		return nil
	}
	raw, ok := req.RequestedSchema.Properties["_approvalMeta"].(map[string]any)
	if !ok {
		return nil
	}
	constValue, _ := raw["const"].(string)
	constValue = strings.TrimSpace(constValue)
	if constValue == "" {
		return nil
	}
	var meta cliApprovalMeta
	if err := json.Unmarshal([]byte(constValue), &meta); err != nil {
		return nil
	}
	if meta.Type != "" && meta.Type != "tool_approval" {
		return nil
	}
	if meta.AcceptLabel == "" {
		meta.AcceptLabel = "Accept"
	}
	if meta.RejectLabel == "" {
		meta.RejectLabel = "Reject"
	}
	if meta.CancelLabel == "" {
		meta.CancelLabel = "Cancel"
	}
	return &meta
}

func awaitToolApprovalElicitation(ctx context.Context, w io.Writer, reader *bufio.Reader, req *coreplan.Elicitation, meta *cliApprovalMeta) (*coreplan.ElicitResult, error) {
	if meta == nil {
		return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionAccept}, nil
	}
	if strings.TrimSpace(meta.Message) != "" && strings.TrimSpace(meta.Message) != strings.TrimSpace(req.Message) {
		fmt.Fprintln(w, meta.Message)
	}
	if toolName := strings.TrimSpace(meta.ToolName); toolName != "" {
		fmt.Fprintf(w, "Tool: %s\n", toolName)
	}
	editedFields := map[string]any{}
	for _, editor := range meta.Editors {
		if editor == nil || strings.TrimSpace(editor.Name) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(editor.Kind)) {
		case "checkbox_list":
			selected, cancel, ctxErr := awaitCheckboxEditor(ctx, w, reader, editor)
			if ctxErr != nil {
				return nil, ctxErr
			}
			if cancel {
				return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
			}
			editedFields[editor.Name] = selected
		case "radio_list":
			selected, cancel, ctxErr := awaitRadioEditor(ctx, w, reader, editor)
			if ctxErr != nil {
				return nil, ctxErr
			}
			if cancel {
				return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
			}
			editedFields[editor.Name] = selected
		}
	}
	for {
		fmt.Fprintf(w, "Submit? [a]%s, [d]%s, [c]%s (default: a): ", meta.AcceptLabel, meta.RejectLabel, meta.CancelLabel)
		line, cancelled, err := readPromptLine(ctx, reader)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				fmt.Fprintln(w, "")
				return nil, err
			}
			log.Printf("[elicitation] read approval input failed: %v", err)
		}
		if cancelled {
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
		}
		sel := strings.ToLower(line)
		switch sel {
		case "", "a", "accept", "allow":
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionAccept, Payload: map[string]any{"editedFields": editedFields}}, nil
		case "d", "decline", "deny", "reject":
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "declined"}, nil
		case "c", "cancel":
			return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
		default:
			fmt.Fprintln(w, "Invalid choice.")
		}
	}
}

func awaitCheckboxEditor(ctx context.Context, w io.Writer, reader *bufio.Reader, editor *cliApprovalEditor) ([]string, bool, error) {
	fmt.Fprintf(w, "\n%s\n", firstNonEmpty(editor.Label, editor.Name))
	if desc := strings.TrimSpace(editor.Description); desc != "" {
		fmt.Fprintln(w, desc)
	}
	for i, option := range editor.Options {
		if option == nil {
			continue
		}
		mark := " "
		if option.Selected {
			mark = "x"
		}
		fmt.Fprintf(w, "  %d. [%s] %s\n", i+1, mark, option.Label)
	}
	fmt.Fprint(w, "Select items to allow (comma-separated numbers, Enter keeps defaults, 'cancel' aborts): ")
	value, cancelled, err := readPromptLine(ctx, reader)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			fmt.Fprintln(w, "")
			return nil, true, err
		}
		log.Printf("[elicitation] read checkbox input failed: %v", err)
	}
	if cancelled {
		return nil, true, nil
	}
	if strings.EqualFold(value, "cancel") {
		return nil, true, nil
	}
	if value == "" {
		selected := make([]string, 0)
		for _, option := range editor.Options {
			if option != nil && option.Selected {
				selected = append(selected, option.ID)
			}
		}
		return selected, false, nil
	}
	selected := make([]string, 0)
	for _, index := range parseSelectionIndexes(value) {
		if index < 1 || index > len(editor.Options) {
			continue
		}
		option := editor.Options[index-1]
		if option == nil {
			continue
		}
		selected = append(selected, option.ID)
	}
	return selected, false, nil
}

func awaitRadioEditor(ctx context.Context, w io.Writer, reader *bufio.Reader, editor *cliApprovalEditor) (string, bool, error) {
	fmt.Fprintf(w, "\n%s\n", firstNonEmpty(editor.Label, editor.Name))
	if desc := strings.TrimSpace(editor.Description); desc != "" {
		fmt.Fprintln(w, desc)
	}
	defaultIndex := 0
	for i, option := range editor.Options {
		if option == nil {
			continue
		}
		mark := " "
		if option.Selected && defaultIndex == 0 {
			mark = "*"
			defaultIndex = i + 1
		}
		fmt.Fprintf(w, "  %d. (%s) %s\n", i+1, mark, option.Label)
	}
	fmt.Fprint(w, "Select one item to allow (number, Enter keeps default, 'cancel' aborts): ")
	value, cancelled, err := readPromptLine(ctx, reader)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			fmt.Fprintln(w, "")
			return "", true, err
		}
		log.Printf("[elicitation] read radio input failed: %v", err)
	}
	if cancelled {
		return "", true, nil
	}
	if strings.EqualFold(value, "cancel") {
		return "", true, nil
	}
	if value == "" {
		if defaultIndex >= 1 && defaultIndex <= len(editor.Options) && editor.Options[defaultIndex-1] != nil {
			return editor.Options[defaultIndex-1].ID, false, nil
		}
		return "", false, nil
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 1 || index > len(editor.Options) || editor.Options[index-1] == nil {
		return "", false, nil
	}
	return editor.Options[index-1].ID, false, nil
}

func parseSelectionIndexes(value string) []int {
	parts := strings.Split(value, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx, err := strconv.Atoi(part); err == nil {
			result = append(result, idx)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
