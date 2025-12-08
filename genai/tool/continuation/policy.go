package continuation

// Package continuation provides a simple policy decision based on tool input
// and output schemas to determine how to handle overflow/pagination.

import (
	sch "github.com/viant/agently/genai/tool/schema"
)

// Strategy indicates how continuation should be handled for a tool call.
type Strategy int

const (
	// NoRanges indicates neither input nor output expose range semantics.
	// Emit YAML overflow wrapper and instruct internal_message-show.
	NoRanges Strategy = iota
	// NativeRanges indicates both output continuation and compatible input ranges
	// are present. Follow nextRange using the matching range kind (bytes/lines).
	NativeRanges
	// OutputOnlyRanges indicates output exposes continuation but inputs do not
	// accept compatible range selectors. Emit YAML wrapper with messageId/nextRange
	// and instruct internal_message-show.
	OutputOnlyRanges
	// InputOnlyRanges indicates inputs accept ranges but outputs don't expose
	// continuation hints. Avoid auto-iteration; only honor explicit user ranges
	// or emit YAML wrapper for autos.
	InputOnlyRanges
)

// Decide returns the strategy based on the presence of continuation in the
// output schema and range selectors in the input schema. Preference is given
// to matching range modalities (bytes with bytes, lines with lines).
func Decide(in sch.RangeInputs, out sch.ContinuationShape) Strategy {
	// Prefer exact modality matches for native ranges
	if out.HasBytes && in.HasBytes {
		return NativeRanges
	}
	if out.HasLines && in.HasLines {
		return NativeRanges
	}
	outAny := out.HasBytes || out.HasLines
	inAny := in.HasBytes || in.HasLines
	switch {
	case outAny && !inAny:
		return OutputOnlyRanges
	case !outAny && inAny:
		return InputOnlyRanges
	case outAny && inAny:
		// Mismatch (e.g., out bytes, in lines). Treat as output-only since
		// we cannot follow the provider hint with available inputs.
		return OutputOnlyRanges
	default:
		return NoRanges
	}
}
