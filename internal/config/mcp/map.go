package mcp

import "github.com/viant/agently/internal/auth/tokens"

// ToTokenStoragePolicy converts config storage policy into tokens.StoragePolicy.
func ToTokenStoragePolicy(sp StoragePolicy) tokens.StoragePolicy {
    return tokens.StoragePolicy{
        AccessInMemoryOnly: sp.Access == "memory" || sp.Access == "",
        IDInMemoryOnly:     sp.ID == "memory" || sp.ID == "",
        RefreshEncrypted:   sp.Refresh != "memory", // encrypted by default
    }
}

