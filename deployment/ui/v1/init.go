package v1

import "embed"

// FS exposes the embedded v1 UI bundle for the app server.
//
//go:embed index.html assets/*
var FS embed.FS

// Index keeps a direct handle to the app shell.
//
//go:embed index.html
var Index []byte
