package gemini

const geminiEndpoint = "https://generativelanguage.googleapis.com/%v/models"

// EMPTY_THOUGHT_SIGNATURE is used as a placeholder prefix when Gemini returns an empty
// thoughtSignature for a functionCall so downstream tooling can still reference a
// non-empty tool call ID (the runtime appends a unique suffix).
const EMPTY_THOUGHT_SIGNATURE = "EMPTY_THOUGHT_SIGNATURE"
