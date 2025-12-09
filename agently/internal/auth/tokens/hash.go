package tokens

import (
	"crypto/sha256"
	"encoding/hex"
)

// realSHA256 returns a hex-encoded SHA-256 hash of data.
// Kept unexported to avoid leaking implementation detail beyond this package.
func realSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
