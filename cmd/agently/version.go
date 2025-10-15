package agently

// version holds the CLI version string. It can be set by the main package
// at startup using SetVersion, which in turn is populated via -ldflags.
// Defaults to "dev" for local builds.
var version = "dev"

// SetVersion initializes the version string if non-empty.
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

// Version returns the current CLI version string.
func Version() string { return version }
