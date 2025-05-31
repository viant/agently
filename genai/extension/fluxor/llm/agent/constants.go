package agent

// Workflow/global map keys and task names used across the llm/agent service.
// Keeping them in one place prevents typos and eases future refactors.

const (
	// Initial / output map keys
	keyQuery = "query"

	keyContext      = "context"
	keyModel        = "model"
	keyTools        = "tools"
	keySystemPrompt = "systemPrompt"
	keyToolPolicy   = "toolPolicy"

	// Workflow post keys / result fields
	keyPlan        = "plan"
	keyResults     = "results"
	keyElicitation = "elicitation"
	keyAnswer      = "answer"

    // Placeholders "${key}" used in workflow templates.
    keyQueryPlaceholder        = "${" + keyQuery + "}"
    keyContextPlaceholder      = "${" + keyContext + "}"
    keyModelPlaceholder        = "${" + keyModel + "}"
    keyToolsPlaceholder        = "${" + keyTools + "}"
    keySystemPromptPlaceholder = "${" + keySystemPrompt + "}"
    keyPlanPlaceholder         = "${" + keyPlan + "}"
    keyResultsPlaceholder      = "${" + keyResults + "}"
    keyElicitationPlaceholder  = "${" + keyElicitation + "}"
    keyAnswerPlaceholder       = "${" + keyAnswer + "}"
)
