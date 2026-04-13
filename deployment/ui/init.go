package ui

import "embed"

// FS exposes the embedded UI bundle for the app server.
//
//go:embed index.html favicon.ico assets/*
var FS embed.FS

// Index keeps a direct handle to the app shell.
//
//go:embed index.html
var Index []byte
