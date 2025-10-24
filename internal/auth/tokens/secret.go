package tokens

import (
    "bytes"
)

// Secret holds sensitive bytes that can be zeroized when cleared.
// Avoid storing tokens as strings to enable zeroization.
type Secret struct {
    b []byte
}

// FromString creates a Secret from a string copy.
func FromString(s string) Secret {
    if s == "" {
        return Secret{}
    }
    out := make([]byte, len(s))
    copy(out, s)
    return Secret{b: out}
}

// Bytes returns a copy of the secret bytes.
func (s Secret) Bytes() []byte {
    if len(s.b) == 0 { return nil }
    out := make([]byte, len(s.b))
    copy(out, s.b)
    return out
}

// String returns a copy as string.
func (s Secret) String() string {
    if len(s.b) == 0 { return "" }
    return string(s.Bytes())
}

// IsEmpty reports whether the secret is empty.
func (s Secret) IsEmpty() bool { return len(s.b) == 0 }

// Clear zeroizes the underlying bytes.
func (s *Secret) Clear() {
    if s == nil || len(s.b) == 0 { return }
    bytes.Fill(s.b, 0)
    s.b = nil
}

