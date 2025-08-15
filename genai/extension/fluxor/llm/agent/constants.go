package agent

// Workflow/global map keys and task names used across the llm/agent service.
// Keeping them in one place prevents typos and eases future refactors.

const (
	// Initial / output map keys
	keyQuery = "query"

	keyContext       = "context"
	keySystemContext = "systemContext"
	keyModel         = "model"
	keyTools         = "tools"
	keySystemPrompt  = "systemPrompt"
	keyToolPolicy    = "toolPolicy"

	// Workflow post keys / result fields
	keyPlan        = "plan"
	keyResults     = "results"
	keyElicitation = "elicitation"
	keyAnswer      = "answer"
	keyToolError   = "toolError"
	keyRefinedPlan = "refinedPlan"
)
