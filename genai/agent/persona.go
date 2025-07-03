package agent

// Persona defines the conversational identity an agent should assume when it
// sends messages.
//
// Role   – chat role string as used by LLM APIs ("assistant", "user", "system").
// Actor  – optional free-text that identifies the actual sender when multiple
//
//	synthetic users exist (e.g. "auto-elicitation"). When empty the UI
//	can hide the badge.
type Persona struct {
	Role  string `yaml:"role,omitempty"  json:"role,omitempty"`
	Actor string `yaml:"actor,omitempty" json:"actor,omitempty"`
}
