package llm

type Matcher interface {
	Best(preferences *ModelPreferences) string
}

// ReducingMatcher optionally allows the caller to reduce the candidate set
// prior to selection using a predicate over model IDs. Implementations should
// apply the filter to their candidate list and then choose the best model as
// if Best() were called on the reduced set.
type ReducingMatcher interface {
	BestWithFilter(preferences *ModelPreferences, allow func(id string) bool) string
}
