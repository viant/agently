package imageio

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"strings"

	_ "image/gif"
)

type Options struct {
	MaxWidth  int
	MaxHeight int
	MaxBytes  int
	Format    string // "", "png", "jpeg"
}

type EncodedImage struct {
	Bytes    []byte
	MimeType string
	Width    int
	Height   int
}

func NormalizeOptions(input Options) Options {
	opts := input
	if opts.MaxWidth <= 0 {
		opts.MaxWidth = 2048
	}
	if opts.MaxHeight <= 0 {
		opts.MaxHeight = 768
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 4 * 1024 * 1024
	}
	opts.Format = strings.ToLower(strings.TrimSpace(opts.Format))
	if opts.Format == "jpg" {
		opts.Format = "jpeg"
	}
	switch opts.Format {
	case "", "png", "jpeg":
	default:
		opts.Format = ""
	}
	return opts
}

func EncodeToFit(raw []byte, options Options) (*EncodedImage, error) {
	opts := NormalizeOptions(options)
	if len(raw) == 0 {
		return nil, fmt.Errorf("image content was empty")
	}
	mimeType := http.DetectContentType(raw)
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return nil, fmt.Errorf("unsupported content type: %s", mimeType)
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image config: %w", err)
	}
	srcWidth := cfg.Width
	srcHeight := cfg.Height
	if srcWidth <= 0 || srcHeight <= 0 {
		return nil, fmt.Errorf("invalid image dimensions")
	}

	img, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	if decodedFormat != "" {
		format = decodedFormat
	}

	targetW, targetH := fitBox(srcWidth, srcHeight, opts.MaxWidth, opts.MaxHeight)
	if targetW <= 0 || targetH <= 0 {
		return nil, fmt.Errorf("invalid target dimensions")
	}
	if targetW != srcWidth || targetH != srcHeight {
		img = resizeNearest(img, targetW, targetH)
	}

	targetFormat := selectOutputFormat(opts.Format, format)
	encodedBytes, targetMime, err := encodeImage(img, targetFormat)
	if err != nil {
		return nil, err
	}
	if opts.MaxBytes > 0 && len(encodedBytes) > opts.MaxBytes {
		return nil, fmt.Errorf("encoded image exceeds maxBytes (%d > %d)", len(encodedBytes), opts.MaxBytes)
	}
	return &EncodedImage{
		Bytes:    encodedBytes,
		MimeType: targetMime,
		Width:    targetW,
		Height:   targetH,
	}, nil
}

func fitBox(width, height, maxWidth, maxHeight int) (int, int) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	if maxWidth <= 0 || maxHeight <= 0 {
		return width, height
	}
	scaleW := float64(maxWidth) / float64(width)
	scaleH := float64(maxHeight) / float64(height)
	scale := math.Min(1.0, math.Min(scaleW, scaleH))
	outW := int(math.Round(float64(width) * scale))
	outH := int(math.Round(float64(height) * scale))
	if outW < 1 {
		outW = 1
	}
	if outH < 1 {
		outH = 1
	}
	return outW, outH
}

func resizeNearest(src image.Image, targetWidth, targetHeight int) image.Image {
	if src == nil {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return dst
	}
	for y := 0; y < targetHeight; y++ {
		srcY := srcBounds.Min.Y + (y*srcH)/targetHeight
		for x := 0; x < targetWidth; x++ {
			srcX := srcBounds.Min.X + (x*srcW)/targetWidth
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func selectOutputFormat(forced, decoded string) string {
	forced = strings.ToLower(strings.TrimSpace(forced))
	switch forced {
	case "png", "jpeg":
		return forced
	}
	decoded = strings.ToLower(strings.TrimSpace(decoded))
	switch decoded {
	case "jpeg", "png":
		return decoded
	default:
		return "png"
	}
}

func encodeImage(img image.Image, format string) ([]byte, string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	var buf bytes.Buffer
	switch format {
	case "jpeg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", fmt.Errorf("encode jpeg: %w", err)
		}
		return buf.Bytes(), "image/jpeg", nil
	default:
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", fmt.Errorf("encode png: %w", err)
		}
		return buf.Bytes(), "image/png", nil
	}
}
