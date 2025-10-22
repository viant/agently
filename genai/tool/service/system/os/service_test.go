package sysos_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	sysos "github.com/viant/agently/genai/tool/service/system/os"
)

func TestService_GetEnv(t *testing.T) {
	// Arrange
	s := sysos.New()
	exec, err := s.Method("getEnv")
	assert.EqualValues(t, nil, err)
	assert.NotNil(t, exec)

	// Ensure environment variable exists
	const key = "AGENTLY_TEST_ENV"
	_ = os.Setenv(key, "abc123")
	defer os.Unsetenv(key)

	in := &sysos.GetEnvInput{Names: []string{key, "MISSING_VAR"}}
	out := &sysos.GetEnvOutput{}

	// Act
	err = exec(context.Background(), in, out)

	// Assert
	assert.EqualValues(t, nil, err)
	assert.EqualValues(t, map[string]string{key: "abc123"}, out.Values)
}
