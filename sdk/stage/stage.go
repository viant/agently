package stage

// Stage captures the live execution status of a conversation turn. All fields
// are optional except Phase.
//
// Clients can display appropriate UI indicators depending on the phase and
// enrich the message stream with workflow/tool progress.
type Stage struct {
	Phase    string `json:"phase"`              // waiting | thinking | executing | done | error
	Workflow string `json:"workflow,omitempty"` // orchestration name/ID
	Task     string `json:"task,omitempty"`     // current workflow step
	Tool     string `json:"tool,omitempty"`     // running tool/function
}

const (
	StageWaiting   = "waiting"
	StageThinking  = "thinking"
	StageExecuting = "executing"
	StageEliciting = "elicitation"
	StageDone      = "done"
	StageError     = "error"
)
