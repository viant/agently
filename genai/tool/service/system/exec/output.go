package exec

// Command represents the result of executing a single command
type Command struct {
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	Stderr string `json:"stderr,omitempty"`
	Status int    `json:"status,omitempty"`
}

// Output represents the results of executing commands
type Output struct {
	Commands []*Command `json:"commands,omitempty"`
	Stdout   string     `json:"stdout,omitempty"`
	Stderr   string     `json:"stderr,omitempty"`
	Status   int        `json:"status,omitempty"`
}
