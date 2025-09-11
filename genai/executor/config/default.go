package config

type Defaults struct {
	Model    string
	Embedder string
	Agent    string

	// ---- Conversation summary defaults (optional) -------------------
	// When empty the runtime falls back to hard-coded defaults.
	SummaryModel  string `yaml:"summaryModel" json:"summaryModel"`
	SummaryPrompt string `yaml:"summaryPrompt" json:"summaryPrompt"`
	SummaryLastN  int    `yaml:"summaryLastN" json:"summaryLastN"`
}
