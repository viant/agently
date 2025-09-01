package domain

import "time"

// Legacy filter types removed in favour of DAO read input options.
// Transcript aggregation options remain below.

// PayloadLevel controls how much of the call payloads are embedded in the
// aggregated transcript. Implementations should honour these semantics best-effort.
type PayloadLevel string

const (
	// PayloadNone excludes payload bodies entirely; IDs/metadata may still be present.
	PayloadNone PayloadLevel = "none"
	// PayloadPreview includes lightweight previews (e.g. small text, summaries) when available.
	PayloadPreview PayloadLevel = "preview"
	// PayloadInlineIfSmall embeds the payload inline when its size is below the configured threshold.
	PayloadInlineIfSmall PayloadLevel = "inline_if_small"
	// PayloadFull attempts to embed full payload bodies regardless of size (use with care).
	PayloadFull PayloadLevel = "full"
)

// TranscriptAggOptions define preferences for building a complete aggregated transcript.
type TranscriptAggOptions struct {
	ExcludeInterim    bool         // omit interim/streaming messages
	IncludeTools      bool         // include tool messages
	IncludeModelCalls bool         // attach model-call aggregates
	IncludeToolCalls  bool         // attach tool-call aggregates
	PayloadLevel      PayloadLevel // how much of payload bodies to include
	PayloadInlineMaxB int          // inline threshold (bytes) when using inline_if_small
	RedactSensitive   bool         // redact secrets/sensitive fields in payloads
	TargetProvider    *string      // optional LLM provider hint for future shaping
	IncludeUsage      bool         // include usage breakdown
	Since             *time.Time   // optional lower bound (createdAt)
	Limit             *int         // optional max messages to include
}
