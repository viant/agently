package sysimage

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/viant/afs"
	svc "github.com/viant/agently/genai/tool/service"
	"github.com/viant/agently/genai/tool/service/shared/imageio"
)

// Name identifies this service for MCP routing.
const Name = "system/image"

type Service struct{}

func New() *Service { return &Service{} }

func (s *Service) Name() string { return Name }

type ReadImageInput struct {
	URI  string `json:"uri,omitempty"`
	Path string `json:"path,omitempty"`

	MaxWidth  int    `json:"maxWidth,omitempty"`
	MaxHeight int    `json:"maxHeight,omitempty"`
	MaxBytes  int    `json:"maxBytes,omitempty"`
	Format    string `json:"format,omitempty"`

	// IncludeData controls whether dataBase64 is returned in the tool response.
	// When false (default), the tool writes the encoded image to EncodedURI and
	// omits dataBase64 to keep tool output small.
	IncludeData bool `json:"includeData,omitempty"`
	// DestURL optionally specifies where to write the encoded image (file://...).
	DestURL string `json:"destURL,omitempty"`
}

type ReadImageOutput struct {
	URI      string `json:"uri"`
	Encoded  string `json:"encodedURI,omitempty"`
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int    `json:"bytes"`
	Base64   string `json:"dataBase64,omitempty"`
}

func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{{
		Name:        "readImage",
		Description: "Read an image from a local path/uri and return an encoded image suitable for vision inputs. Defaults to resizing to fit 2048x768. By default it writes the encoded image to encodedURI and omits dataBase64 to keep tool output small; set includeData=true to return base64.",
		Input:       reflect.TypeOf(&ReadImageInput{}),
		Output:      reflect.TypeOf(&ReadImageOutput{}),
	}}
}

func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "readimage":
		return s.readImage, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

func (s *Service) readImage(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ReadImageInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*ReadImageOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	target, err := resolveTarget(input)
	if err != nil {
		return err
	}
	raw, err := afs.New().DownloadWithURL(ctx, target)
	if err != nil {
		return err
	}
	options := imageio.NormalizeOptions(imageio.Options{
		MaxWidth:  input.MaxWidth,
		MaxHeight: input.MaxHeight,
		MaxBytes:  input.MaxBytes,
		Format:    strings.TrimSpace(input.Format),
	})
	encoded, err := imageio.EncodeToFit(raw, options)
	if err != nil {
		return err
	}
	output.URI = target
	output.Path = strings.TrimSpace(input.Path)
	if output.Path == "" {
		output.Path = target
	}
	output.Name = path.Base(output.Path)
	output.MimeType = encoded.MimeType
	output.Width = encoded.Width
	output.Height = encoded.Height
	output.Bytes = len(encoded.Bytes)
	encodedURI, err := imageio.StoreEncodedImage(ctx, encoded, imageio.StoreOptions{DestURL: strings.TrimSpace(input.DestURL)})
	if err != nil {
		return err
	}
	output.Encoded = encodedURI
	if input.IncludeData {
		output.Base64 = base64.StdEncoding.EncodeToString(encoded.Bytes)
	}
	return nil
}

func resolveTarget(input *ReadImageInput) (string, error) {
	if input == nil {
		return "", fmt.Errorf("input was nil")
	}
	if u := strings.TrimSpace(input.URI); u != "" {
		return u, nil
	}
	if p := strings.TrimSpace(input.Path); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("uri or path is required")
}
