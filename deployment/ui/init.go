package ui

import "embed"

//go:embed *
var FS embed.FS

//go:embed index.html
var Index []byte

func init() {}
