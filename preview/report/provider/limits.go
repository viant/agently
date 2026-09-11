package preview

// Limits can only lower the default host resource limits. Zero selects the default.
type Limits struct {
	ReportBytes        int64
	FixtureBytes       int64
	Rows               int
	Columns            int
	PredicateLeaves    int
	GroupingDimensions int
	PageSize           int
	ConcurrentQueries  int
}

func (l Limits) normalized() (Limits, error) {
	defaults := Limits{2 << 20, 25 << 20, 100000, 500, 100, 20, 1000, 8}
	for _, pair := range []struct {
		value *int
		max   int
	}{{&l.Rows, defaults.Rows}, {&l.Columns, defaults.Columns}, {&l.PredicateLeaves, defaults.PredicateLeaves}, {&l.GroupingDimensions, defaults.GroupingDimensions}, {&l.PageSize, defaults.PageSize}, {&l.ConcurrentQueries, defaults.ConcurrentQueries}} {
		if *pair.value == 0 {
			*pair.value = pair.max
		}
		if *pair.value < 1 || *pair.value > pair.max {
			return l, fail("LimitExceeded", "", "limits", "host limits must be positive and no greater than defaults")
		}
	}
	for _, pair := range []struct {
		value *int64
		max   int64
	}{{&l.ReportBytes, defaults.ReportBytes}, {&l.FixtureBytes, defaults.FixtureBytes}} {
		if *pair.value == 0 {
			*pair.value = pair.max
		}
		if *pair.value < 1 || *pair.value > pair.max {
			return l, fail("LimitExceeded", "", "limits", "host limits must be positive and no greater than defaults")
		}
	}
	return l, nil
}
