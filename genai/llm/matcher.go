package llm

type Matcher interface {
	Best(preferences *ModelPreferences) string
}
