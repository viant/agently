package agently

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/viant/agently-core/protocol/prompt"
)

const scyInlineBase64Prefix = "inlined://base64/"

func parseContextArg(raw string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	data := []byte(raw)
	if strings.HasPrefix(raw, "@") {
		content, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return nil, fmt.Errorf("read context file: %w", err)
		}
		data = content
	}
	result := map[string]interface{}{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse context JSON: %w", err)
	}
	return result, nil
}

func parseAttachments(values []string) ([]*prompt.Attachment, error) {
	var out []*prompt.Attachment
	for _, item := range values {
		path := strings.TrimSpace(item)
		if path == "" {
			return nil, fmt.Errorf("attachment path is required")
		}
		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if strings.TrimSpace(mimeType) == "" {
			return nil, fmt.Errorf("unsupported attachment type for %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", path, err)
		}
		out = append(out, &prompt.Attachment{
			Name: filepath.Base(path),
			Mime: mimeType,
			Data: data,
		})
	}
	return out, nil
}

func resolvedToken(flagValue string) string {
	if token := strings.TrimSpace(flagValue); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("AGENTLY_TOKEN"))
}

func parseJSONArg(raw string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	data := []byte(raw)
	if strings.HasPrefix(raw, "@") {
		content, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return nil, err
		}
		data = content
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func cliCookieJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return jar
}

func normalizeCLIContent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func inlineLocalScyResource(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("empty scy resource")
	}
	if strings.HasPrefix(value, scyInlineBase64Prefix) {
		return value, nil
	}
	parts := strings.SplitN(value, "|", 2)
	resourceURL := strings.TrimSpace(parts[0])
	resourceKey := ""
	if len(parts) == 2 {
		resourceKey = strings.TrimSpace(parts[1])
	}
	if resourceURL == "" {
		return "", fmt.Errorf("empty scy resource url")
	}
	if isNonFileResourceURL(resourceURL) {
		return value, nil
	}
	if strings.HasPrefix(resourceURL, "~") {
		resourceURL = os.Getenv("HOME") + resourceURL[1:]
	} else if strings.HasPrefix(resourceURL, "/~") {
		resourceURL = os.Getenv("HOME") + resourceURL[2:]
	}
	if strings.HasPrefix(resourceURL, "file://") {
		resourceURL = strings.TrimPrefix(resourceURL, "file://")
	}
	payload, err := os.ReadFile(resourceURL)
	if err != nil {
		return "", fmt.Errorf("read local oob secret %q: %w", resourceURL, err)
	}
	inlineURL := scyInlineBase64Prefix + base64.StdEncoding.EncodeToString(payload)
	if resourceKey != "" {
		inlineURL += "|" + resourceKey
	}
	return inlineURL, nil
}

func isNonFileResourceURL(raw string) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "inlined://") {
		return true
	}
	for _, prefix := range []string{
		"gcp://", "aws://", "s3://", "gs://", "http://", "https://", "scp://", "ssh://", "mem://", "secret://",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func parseScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}
