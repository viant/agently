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
