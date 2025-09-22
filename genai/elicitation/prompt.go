package elicitation

import (
	"context"
	plan "github.com/viant/agently/genai/agent/plan"
	stdioprompt "github.com/viant/agently/genai/elicitation/stdio"
	"io"
)

func Prompt(ctx context.Context, w io.Writer, r io.Reader, p *plan.Elicitation) (*plan.ElicitResult, error) {
	return stdioprompt.Prompt(ctx, w, r, p)
}
