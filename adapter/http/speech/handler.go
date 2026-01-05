package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

type Response struct {
	Text string `json:"text"`
}

type openAITranscriptionResponse struct {
	Text string `json:"text"`
}

const defaultMaxUploadBytes = 25 << 20 // 25MB

func maxUploadBytes() int64 {
	v := strings.TrimSpace(os.Getenv("AGENTLY_SPEECH_MAX_BYTES"))
	if v == "" {
		return defaultMaxUploadBytes
	}
	if n, err := parseInt64(v); err == nil && n > 0 {
		return n
	}
	return defaultMaxUploadBytes
}

func parseInt64(v string) (int64, error) {
	var out int64
	_, err := fmt.Sscan(v, &out)
	return out, err
}

func openAIModel() string {
	v := strings.TrimSpace(os.Getenv("AGENTLY_SPEECH_OPENAI_MODEL"))
	if v == "" {
		return "whisper-1"
	}
	return v
}

func openAIBaseURL() string {
	v := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if v == "" {
		return "https://api.openai.com"
	}
	return strings.TrimRight(v, "/")
}

func openAIKey() string {
	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// NewHandler returns a simple /v1/api/speech/transcribe handler.
// It expects multipart/form-data with field "file" (audio).
// It returns JSON: {"text": "..."}.
func NewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if openAIKey() == "" {
			writeError(w, http.StatusNotImplemented, "speech-to-text is not configured (missing OPENAI_API_KEY)")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes())
		if err := r.ParseMultipartForm(maxUploadBytes()); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid multipart payload: %v", err))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing form field: file")
			return
		}
		defer func() { _ = file.Close() }()

		text, err := transcribeOpenAI(r.Context(), file, header)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, Response{Text: text})
	}
}

func transcribeOpenAI(ctx context.Context, file multipart.File, header *multipart.FileHeader) (string, error) {
	key := openAIKey()
	if key == "" {
		return "", fmt.Errorf("speech-to-text is not configured")
	}
	model := openAIModel()
	base := openAIBaseURL()
	url := base + "/v1/audio/transcriptions"

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer func() { _ = pw.Close() }()
		defer func() { _ = writer.Close() }()

		if err := writer.WriteField("model", model); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if err := writer.WriteField("response_format", "json"); err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		filename := "audio.webm"
		if header != nil && strings.TrimSpace(header.Filename) != "" {
			filename = header.Filename
		}
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai transcription request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai transcription read failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai transcription error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	out := openAITranscriptionResponse{}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("openai transcription parse failed: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}
