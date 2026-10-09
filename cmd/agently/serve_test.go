package agently

import (
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

func TestServeCmd_ScratchpadRootURI(t *testing.T) {
	for _, flag := range []string{"-s", "--scratchpad-root-uri"} {
		t.Run(flag, func(t *testing.T) {
			cmd := &ServeCmd{}
			parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)

			_, err := parser.ParseArgs([]string{
				flag, "gs://scratchpad-bucket/users/${userID}",
			})
			require.NoError(t, err)
			require.Equal(t, "gs://scratchpad-bucket/users/${userID}", cmd.ScratchpadRootURI)
			require.Equal(t, cmd.ScratchpadRootURI, cmd.serveOptions().ScratchpadRootURI)
		})
	}
}

func TestServeCmd_AuthorizationConfig(t *testing.T) {
	cmd := &ServeCmd{}
	parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)
	_, err := parser.ParseArgs([]string{"--authz-config", "/operator/authz.json"})
	require.NoError(t, err)
	require.Equal(t, "/operator/authz.json", cmd.serveOptions().AuthzConfigPath)
}

func TestServeCmd_AuthorizationValidationIsExplicit(t *testing.T) {
	cmd := &ServeCmd{}
	parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)
	_, err := parser.ParseArgs([]string{"--validate-authz", "--authz-config", "/operator/authz.json", "--workspace", "/operator/workspace"})
	require.NoError(t, err)
	require.True(t, cmd.ValidateAuthz)
	require.Equal(t, "/operator/workspace", cmd.serveOptions().WorkspacePath)
}

func TestServeCmd_BackgroundWorkersAreExplicitlyDisabled(t *testing.T) {
	cmd := &ServeCmd{}
	parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)
	_, err := parser.ParseArgs([]string{"--disable-background-workers"})
	require.NoError(t, err)
	require.True(t, cmd.serveOptions().DisableBackgroundWorkers)
}
