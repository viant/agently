package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	coreplan "github.com/viant/agently-core/protocol/agent/plan"
)

func awaitCoreElicitation(_ context.Context, req *coreplan.Elicitation) (*coreplan.ElicitResult, error) {
	if req == nil || req.IsEmpty() {
		return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionAccept}, nil
	}
	return awaitFormElicitation(os.Stdout, os.Stdin, req)
}

func awaitFormElicitation(w io.Writer, r io.Reader, req *coreplan.Elicitation) (*coreplan.ElicitResult, error) {
	reader := bufio.NewReader(r)

	fmt.Fprintf(w, "\n--- Elicitation ---\n%s\n", req.Message)
	if meta := parseToolApprovalMeta(req); meta != nil {
		return awaitToolApprovalElicitation(w, reader, req, meta)
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
		line, _ := reader.ReadString('\n')
		value := strings.TrimSpace(line)
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
		line, _ := reader.ReadString('\n')
		sel := strings.ToLower(strings.TrimSpace(line))
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

func awaitToolApprovalElicitation(w io.Writer, reader *bufio.Reader, req *coreplan.Elicitation, meta *cliApprovalMeta) (*coreplan.ElicitResult, error) {
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
			selected, cancel := awaitCheckboxEditor(w, reader, editor)
			if cancel {
				return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
			}
			editedFields[editor.Name] = selected
		case "radio_list":
			selected, cancel := awaitRadioEditor(w, reader, editor)
			if cancel {
				return &coreplan.ElicitResult{Action: coreplan.ElicitResultActionDecline, Reason: "cancelled"}, nil
			}
			editedFields[editor.Name] = selected
		}
	}
	for {
		fmt.Fprintf(w, "Submit? [a]%s, [d]%s, [c]%s (default: a): ", meta.AcceptLabel, meta.RejectLabel, meta.CancelLabel)
		line, _ := reader.ReadString('\n')
		sel := strings.ToLower(strings.TrimSpace(line))
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

func awaitCheckboxEditor(w io.Writer, reader *bufio.Reader, editor *cliApprovalEditor) ([]string, bool) {
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
	line, _ := reader.ReadString('\n')
	value := strings.TrimSpace(line)
	if strings.EqualFold(value, "cancel") {
		return nil, true
	}
	if value == "" {
		selected := make([]string, 0)
		for _, option := range editor.Options {
			if option != nil && option.Selected {
				selected = append(selected, option.ID)
			}
		}
		return selected, false
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
	return selected, false
}

func awaitRadioEditor(w io.Writer, reader *bufio.Reader, editor *cliApprovalEditor) (string, bool) {
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
	line, _ := reader.ReadString('\n')
	value := strings.TrimSpace(line)
	if strings.EqualFold(value, "cancel") {
		return "", true
	}
	if value == "" {
		if defaultIndex >= 1 && defaultIndex <= len(editor.Options) && editor.Options[defaultIndex-1] != nil {
			return editor.Options[defaultIndex-1].ID, false
		}
		return "", false
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 1 || index > len(editor.Options) || editor.Options[index-1] == nil {
		return "", false
	}
	return editor.Options[index-1].ID, false
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
