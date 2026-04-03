package agently

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
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
