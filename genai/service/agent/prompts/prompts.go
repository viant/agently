package prompts

import _ "embed"

//go:embed summary.md
var Summary string

//go:embed compact.md
var Compact string

//go:embed prune_prompt.md
var Prune string
