package scheduler

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_parseCronField_DataDriven(t *testing.T) {
	type tc struct {
		name     string
		expr     string
		min, max int
		contains []int
		wantErr  bool
	}
	cases := []tc{
		{name: "any", expr: "*", min: 0, max: 5, contains: []int{0, 1, 2, 3, 4, 5}},
		{name: "single", expr: "3", min: 0, max: 5, contains: []int{3}},
		{name: "range", expr: "1-3", min: 0, max: 5, contains: []int{1, 2, 3}},
		{name: "list", expr: "0,2,4", min: 0, max: 5, contains: []int{0, 2, 4}},
		{name: "step-any", expr: "*/2", min: 0, max: 5, contains: []int{0, 2, 4}},
		{name: "step-range", expr: "1-5/2", min: 0, max: 6, contains: []int{1, 3, 5}},
		{name: "invalid", expr: "a-b", min: 0, max: 5, wantErr: true},
	}
	for _, c := range cases {
		got, err := parseCronField(c.expr, c.min, c.max)
		if c.wantErr {
			assert.NotNil(t, err, c.name)
			continue
		}
		assert.Nil(t, err, c.name)
		for _, v := range c.contains {
			assert.EqualValues(t, true, got[v], c.name)
		}
	}
}

func Test_cronNext_DataDriven(t *testing.T) {
	type tc struct {
		name string
		expr string
		from time.Time
		want time.Time
	}
	base := time.Date(2025, 10, 3, 9, 0, 0, 0, time.UTC)
	cases := []tc{
		{name: "every_minute", expr: "*/1 * * * *", from: base, want: base.Add(time.Minute)},
		{name: "hourly_at_05", expr: "5 * * * *", from: base, want: time.Date(2025, 10, 3, 9, 5, 0, 0, time.UTC)},
		{name: "daily_midnight", expr: "0 0 * * *", from: base, want: time.Date(2025, 10, 4, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		spec, err := parseCron(c.expr)
		assert.Nil(t, err, c.name)
		got := cronNext(spec, c.from)
		assert.EqualValues(t, c.want, got, c.name)
	}
}
