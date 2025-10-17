package system

// Host describes a target system endpoint and credential reference.
type Host struct {
	URL         string `yaml:"URL" json:"url"`
	Credentials string `json:"credentials,omitempty" yaml:"credentials,omitempty"`
}
