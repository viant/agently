package redact

import (
    "encoding/json"
    "os"
    "strings"
)

var defaultKeys []string

func init() {
    // Default sensitive keys; can be overridden via AGENTLY_REDACT_KEYS (comma-separated)
    env := strings.TrimSpace(os.Getenv("AGENTLY_REDACT_KEYS"))
    if env != "" {
        parts := strings.Split(env, ",")
        for i := range parts { parts[i] = strings.ToLower(strings.TrimSpace(parts[i])) }
        defaultKeys = parts
        return
    }
    defaultKeys = []string{
        "api_key", "apikey", "authorization", "auth", "password", "passwd", "secret", "token", "bearer", "client_secret",
    }
}

func DefaultKeys() []string { return append([]string(nil), defaultKeys...) }

// ScrubJSONBytes removes/sanitizes values of the provided keys (case-insensitive) in a JSON document.
func ScrubJSONBytes(data []byte, keys []string) []byte {
    if len(data) == 0 { return data }
    if len(keys) == 0 { keys = defaultKeys }
    m := make(map[string]struct{}, len(keys))
    for _, k := range keys { m[strings.ToLower(strings.TrimSpace(k))] = struct{}{} }
    var v interface{}
    if err := json.Unmarshal(data, &v); err != nil {
        return data
    }
    v = scrubValue(v, m)
    out, err := json.Marshal(v)
    if err != nil { return data }
    return out
}

func scrubValue(v interface{}, keys map[string]struct{}) interface{} {
    switch t := v.(type) {
    case map[string]interface{}:
        out := make(map[string]interface{}, len(t))
        for k, val := range t {
            if _, ok := keys[strings.ToLower(k)]; ok {
                out[k] = "***REDACTED***"
                continue
            }
            out[k] = scrubValue(val, keys)
        }
        return out
    case []interface{}:
        for i := range t {
            t[i] = scrubValue(t[i], keys)
        }
        return t
    default:
        return v
    }
}

