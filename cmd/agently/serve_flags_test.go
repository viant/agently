package agently

import (
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestServeCmd_ExposeMCPFlag_DataDriven(t *testing.T) {
	type testCase struct {
		name       string
		args       []string
		expectFlag bool
	}

	cases := []testCase{
		{
			name:       "default disabled",
			args:       []string{},
			expectFlag: false,
		},
		{
			name:       "enabled with flag",
			args:       []string{"--expose-mcp"},
			expectFlag: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &ServeCmd{}
			parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)
			_, err := parser.ParseArgs(tc.args)
			assert.EqualValues(t, nil, err)
			assert.EqualValues(t, tc.expectFlag, cmd.ExposeMCP)
		})
	}
}

func TestServeCmd_WorkspaceFlag(t *testing.T) {
	cmd := &ServeCmd{}
	parser := flags.NewParser(cmd, flags.HelpFlag|flags.PassDoubleDash)
	_, err := parser.ParseArgs([]string{"--workspace", "/tmp/agently-workspace"})
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/agently-workspace", cmd.Workspace)
}

func TestServeTarget(t *testing.T) {
	type testCase struct {
		name      string
		args      []string
		expect    string
		expectErr bool
	}

	cases := []testCase{
		{name: "default target", args: nil, expect: "legacy"},
		{name: "empty target", args: []string{""}, expect: "legacy"},
		{name: "v1 target", args: []string{"v1"}, expect: "v1"},
		{name: "legacy target", args: []string{"legacy"}, expect: "legacy"},
		{name: "unknown target", args: []string{"v2"}, expectErr: true},
		{name: "extra args after v1", args: []string{"v1", "extra"}, expectErr: true},
		{name: "extra args after legacy", args: []string{"legacy", "extra"}, expectErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := serveTarget(tc.args)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expect, actual)
		})
	}
}

func TestOptionsInit_QueryAlias(t *testing.T) {
	opts := &Options{}
	opts.Init("query")
	assert.NotNil(t, opts.Query)
	assert.Nil(t, opts.Chat)
}
