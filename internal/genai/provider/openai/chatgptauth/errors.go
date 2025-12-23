package chatgptauth

import "fmt"

type TokenStateNotFoundError struct {
	TokensURL string
}

func (e *TokenStateNotFoundError) Error() string {
	if e == nil {
		return "token state not found"
	}
	if e.TokensURL == "" {
		return "token state not found"
	}
	return fmt.Sprintf("no ChatGPT OAuth token state found at %s", e.TokensURL)
}

// MissingOrganizationIDError indicates the ChatGPT-issued ID token cannot be used to mint
// an OpenAI API key because it is missing an `organization_id` claim.
type MissingOrganizationIDError struct {
	Message string
}

func (e *MissingOrganizationIDError) Error() string {
	if e == nil {
		return "invalid id token: missing organization_id"
	}
	if e.Message != "" {
		return e.Message
	}
	return "invalid id token: missing organization_id"
}
