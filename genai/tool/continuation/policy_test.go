package continuation

import (
	"github.com/stretchr/testify/assert"
	sch "github.com/viant/agently/genai/tool/schema"
	"testing"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name string
		in   sch.RangeInputs
		out  sch.ContinuationShape
		want Strategy
	}{
		{name: "bytes native", in: sch.RangeInputs{HasBytes: true}, out: sch.ContinuationShape{HasBytes: true}, want: NativeRanges},
		{name: "lines native", in: sch.RangeInputs{HasLines: true}, out: sch.ContinuationShape{HasLines: true}, want: NativeRanges},
		{name: "both out; bytes in", in: sch.RangeInputs{HasBytes: true}, out: sch.ContinuationShape{HasBytes: true, HasLines: true}, want: NativeRanges},
		{name: "both out; lines in", in: sch.RangeInputs{HasLines: true}, out: sch.ContinuationShape{HasBytes: true, HasLines: true}, want: NativeRanges},
		{name: "mismatch out bytes; in lines", in: sch.RangeInputs{HasLines: true}, out: sch.ContinuationShape{HasBytes: true}, want: OutputOnlyRanges},
		{name: "mismatch out lines; in bytes", in: sch.RangeInputs{HasBytes: true}, out: sch.ContinuationShape{HasLines: true}, want: OutputOnlyRanges},
		{name: "output only", in: sch.RangeInputs{}, out: sch.ContinuationShape{HasBytes: true}, want: OutputOnlyRanges},
		{name: "input only bytes", in: sch.RangeInputs{HasBytes: true}, out: sch.ContinuationShape{}, want: InputOnlyRanges},
		{name: "input only lines", in: sch.RangeInputs{HasLines: true}, out: sch.ContinuationShape{}, want: InputOnlyRanges},
		{name: "none", in: sch.RangeInputs{}, out: sch.ContinuationShape{}, want: NoRanges},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.in, tc.out)
			assert.EqualValues(t, tc.want, got)
		})
	}
}
