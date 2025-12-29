package sysimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_ReadImage_OutputContract_DataDriven(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	raw := buf.Bytes()

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.png")
	require.NoError(t, os.WriteFile(srcPath, raw, 0644))
	srcURI := "file://" + srcPath

	type testCase struct {
		name        string
		input       *ReadImageInput
		assertFn    func(t *testing.T, out *ReadImageOutput, err error)
		expectError bool
	}

	service := New()
	ctx := context.Background()

	testCases := []testCase{
		{
			name: "includeData false writes encodedURI and omits base64",
			input: &ReadImageInput{
				URI:         srcURI,
				IncludeData: false,
			},
			assertFn: func(t *testing.T, out *ReadImageOutput, err error) {
				assert.EqualValues(t, nil, err)
				assert.EqualValues(t, srcURI, out.URI)
				assert.EqualValues(t, "", out.Base64)
				assert.EqualValues(t, true, strings.HasPrefix(out.Encoded, "file://"))
				assert.EqualValues(t, true, out.Bytes > 0)
				_, statErr := os.Stat(strings.TrimPrefix(out.Encoded, "file://"))
				assert.EqualValues(t, nil, statErr)
			},
		},
		{
			name: "includeData true returns base64",
			input: &ReadImageInput{
				URI:         srcURI,
				IncludeData: true,
			},
			assertFn: func(t *testing.T, out *ReadImageOutput, err error) {
				assert.EqualValues(t, nil, err)
				assert.EqualValues(t, srcURI, out.URI)
				assert.EqualValues(t, true, strings.TrimSpace(out.Base64) != "")
				raw, decErr := base64.StdEncoding.DecodeString(out.Base64)
				assert.EqualValues(t, nil, decErr)
				assert.EqualValues(t, out.Bytes, len(raw))
				assert.EqualValues(t, true, strings.HasPrefix(out.Encoded, "file://"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := &ReadImageOutput{}
			err := service.readImage(ctx, tc.input, out)
			if tc.expectError {
				assert.EqualValues(t, true, err != nil)
			}
			if tc.assertFn != nil {
				tc.assertFn(t, out, err)
			}
		})
	}
}
