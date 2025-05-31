package optconv

import (
    "strings"
    "github.com/viant/agently/genai/llm"
)

// TemperaturePtr returns a pointer to Temperature value that should be sent to provider.
// Rules:
// 1. If opts == nil -> nil
// 2. If model id starts with "o4-" -> nil (model accepts only default)
// 3. If Temperature == 1 (default) -> nil
// 4. Otherwise returns pointer to opts.Temperature (valid even for 0)
func TemperaturePtr(opts *llm.Options) *float64 {
    if opts == nil {
        return nil
    }

    if strings.HasPrefix(opts.Model, "o4-") {
        return nil
    }

    if opts.Temperature == 1 {
        return nil
    }

    value := opts.Temperature
    return &value
}
