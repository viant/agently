package server

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

type speechResponse struct {
	Text string `json:"text"`
}

type openAITranscriptionResponse struct {
	Text string `json:"text"`
}

const defaultMaxUploadBytes = 25 << 20

func NewSpeechHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if openAIKey() == "" {
			writeSpeechError(w, http.StatusNotImplemented, "speech-to-text is not configured (missing OPENAI_API_KEY)")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, speechMaxUploadBytes())
		if err := r.ParseMultipartForm(speechMaxUploadBytes()); err != nil {
			writeSpeechError(w, http.StatusBadRequest, fmt.Sprintf("invalid multipart payload: %v", err))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeSpeechError(w, http.StatusBadRequest, "missing form field: file")
			return
		}
		defer func() { _ = file.Close() }()

		text, err := transcribeOpenAI(r.Context(), file, header)
		if err != nil {
			writeSpeechError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeSpeechJSON(w, http.StatusOK, speechResponse{Text: text})
	}
}

func speechMaxUploadBytes() int64 {
	v := strings.TrimSpace(os.Getenv("AGENTLY_SPEECH_MAX_BYTES"))
	if v == "" {
		return defaultMaxUploadBytes
	}
	var out int64
	if _, err := fmt.Sscan(v, &out); err == nil && out > 0 {
		return out
	}
	return defaultMaxUploadBytes
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

func writeSpeechJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSpeechError(w http.ResponseWriter, status int, message string) {
	writeSpeechJSON(w, status, map[string]string{"error": message})
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
