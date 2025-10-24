package shared

import (
    "context"
    "testing"
)

// Test helpers
func boolPtr(v bool) *bool { return &v }
func strPtr(s string) *string { return &s }

func assertEqualValues[T comparable](t *testing.T, expected, actual T, msg string) {
    if expected != actual {
        t.Fatalf("%s: expected %v, got %v", msg, expected, actual)
    }
}

func TestResolveReuseAuthorizer_Precedence(t *testing.T) {
    bg := context.Background()
    cases := []struct {
        name string
        in   ReuseAuthorizerResolutionInput
        want bool
    }{
        {"all nil -> built-in false", ReuseAuthorizerResolutionInput{Ctx: bg}, false},
        {"provider=false -> false", ReuseAuthorizerResolutionInput{Ctx: bg, ProviderOpt: boolPtr(false)}, false},
        {"global=false, provider=nil -> false", ReuseAuthorizerResolutionInput{Ctx: bg, GlobalOpt: boolPtr(false)}, false},
        {"cli=true overrides provider=false", ReuseAuthorizerResolutionInput{Ctx: bg, ProviderOpt: boolPtr(false), CLIOpt: boolPtr(true)}, true},
        {"env=true overrides provider=false", ReuseAuthorizerResolutionInput{Ctx: bg, ProviderOpt: boolPtr(false), EnvOpt: boolPtr(true)}, true},
        {"client=false overrides cli=true", ReuseAuthorizerResolutionInput{Ctx: bg, ClientOpt: boolPtr(false), CLIOpt: boolPtr(true)}, false},
        {"context true overrides client=false", ReuseAuthorizerResolutionInput{Ctx: WithReuseAuthorizer(bg, true), ClientOpt: boolPtr(false)}, true},
    }
    for _, tc := range cases {
        got := ResolveReuseAuthorizer(tc.in)
        assertEqualValues(t, tc.want, got, tc.name)
    }
}

func TestResolveReuseAuthorizerMode_PrecedenceAndNormalization(t *testing.T) {
    bg := context.Background()

    cases := []struct {
        name string
        in   ReuseModeResolutionInput
        want Mode
    }{
        {"all nil -> built-in bearer_first", ReuseModeResolutionInput{Ctx: bg}, ModeBearerFirst},
        {"global cookie_first", ReuseModeResolutionInput{Ctx: bg, GlobalOpt: strPtr("cookie_first")}, ModeCookieFirst},
        {"provider bearer-first hyphen", ReuseModeResolutionInput{Ctx: bg, ProviderOpt: strPtr("bearer-first"), GlobalOpt: strPtr("cookie_first")}, ModeBearerFirst},
        {"env invalid ignored -> provider", ReuseModeResolutionInput{Ctx: bg, EnvOpt: strPtr("invalid"), ProviderOpt: strPtr("cookie_first")}, ModeCookieFirst},
        {"cli overrides provider", ReuseModeResolutionInput{Ctx: bg, CLIOpt: strPtr("bearer_first"), ProviderOpt: strPtr("cookie_first")}, ModeBearerFirst},
        {"client overrides cli", ReuseModeResolutionInput{Ctx: bg, ClientOpt: strPtr("cookie_first"), CLIOpt: strPtr("bearer_first")}, ModeCookieFirst},
        {"context overrides client", ReuseModeResolutionInput{Ctx: WithReuseAuthorizerMode(bg, ModeBearerFirst), ClientOpt: strPtr("cookie_first")}, ModeBearerFirst},
    }
    for _, tc := range cases {
        got := ResolveReuseAuthorizerMode(tc.in)
        assertEqualValues(t, tc.want, got, tc.name)
    }
}
