package agently

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveTokensURLFromClientURL(t *testing.T) {
	type testCase struct {
		name      string
		clientURL string
		expected  string
		wantErr   bool
	}

	testCases := []testCase{
		{name: "json path", clientURL: "/Users/me/.secret/gpt.json", expected: "/Users/me/.secret/gpt.tokens.json"},
		{name: "yaml path", clientURL: "/Users/me/.secret/gpt.yaml", expected: "/Users/me/.secret/gpt.tokens.yaml"},
		{name: "no ext", clientURL: "/Users/me/.secret/gpt", expected: "/Users/me/.secret/gpt.tokens.json"},
		{name: "with key suffix", clientURL: "/Users/me/.secret/gpt.json|blowfish://default", expected: "/Users/me/.secret/gpt.tokens.json|blowfish://default"},
		{name: "file url", clientURL: "file:///Users/me/.secret/gpt.json", expected: "file:///Users/me/.secret/gpt.tokens.json"},
		{name: "file url with suffix", clientURL: "file:///Users/me/.secret/gpt.json|blowfish://default", expected: "file:///Users/me/.secret/gpt.tokens.json|blowfish://default"},
		{name: "empty", clientURL: "", expected: "", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deriveTokensURLFromClientURL(tc.clientURL)
			if tc.wantErr {
				assert.EqualValues(t, true, err != nil)
				return
			}
			assert.EqualValues(t, nil, err)
			assert.EqualValues(t, tc.expected, got)
		})
	}
}

func TestParseCallbackPort(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"integer", "1455", 1455, false},
		{"integer with padding", "  9090  ", 9090, false},
		{"auto lowercase", "auto", 0, false},
		{"auto uppercase", "AUTO", 0, false},
		{"zero as auto", "0", 0, false},
		{"negative", "-1", 0, true},
		{"too large", "99999", 0, true},
		{"non-numeric", "hello", 0, true},
	}
	for _, tc := range cases {
		got, err := parseCallbackPort(tc.in)
		if tc.wantErr {
			require.Error(t, err, tc.name)
			continue
		}
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.want, got, tc.name)
	}
}

func TestResolveCallbackPort_Precedence(t *testing.T) {
	// flag wins over env wins over baked-in 1455 default.
	t.Setenv("AGENTLY_CHATGPT_CALLBACK_PORT", "")
	got, err := resolveCallbackPort("")
	require.NoError(t, err)
	assert.Equal(t, 1455, got, "default when flag and env both empty")

	t.Setenv("AGENTLY_CHATGPT_CALLBACK_PORT", "9090")
	got, err = resolveCallbackPort("")
	require.NoError(t, err)
	assert.Equal(t, 9090, got, "env used when flag empty")

	got, err = resolveCallbackPort("7777")
	require.NoError(t, err)
	assert.Equal(t, 7777, got, "flag overrides env")

	got, err = resolveCallbackPort("auto")
	require.NoError(t, err)
	assert.Equal(t, 0, got, "auto resolves to 0 (OS-picked)")
}

func TestListenWithCancel_AutoPortPicksFreePort(t *testing.T) {
	listener, port, err := listenWithCancel(0)
	require.NoError(t, err)
	defer listener.Close()
	require.NotNil(t, listener)
	assert.Greater(t, port, 0, "OS should allocate a non-zero port")
	addr := listener.Addr().(*net.TCPAddr)
	assert.Equal(t, "127.0.0.1", addr.IP.String())
	assert.Equal(t, port, addr.Port)
}
