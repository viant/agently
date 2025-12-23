package chatgptauth

// Options configures ChatGPT OAuth (Code+PKCE) based credential acquisition.
type Options struct {
	ClientURL          string
	TokensURL          string
	Issuer             string
	AllowedWorkspaceID string
	Originator         string
}
