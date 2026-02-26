package modelcallctx

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	gfread "github.com/viant/agently/pkg/agently/generatedfile/read"
)

const (
	envOpenAIGeneratedFileEnabled  = "AGENTLY_OPENAI_GENERATED_FILE_ENABLED"
	envOpenAIGeneratedFileCopyMode = "AGENTLY_OPENAI_GENERATED_FILE_COPY_MODE"
)

type openAIGeneratedFileRef struct {
	Mode           string
	ContainerID    string
	ProviderFileID string
	Filename       string
	MimeType       string
	SizeBytes      int
	Checksum       string
	InlineBody     []byte
}

func (o *recorderObserver) persistOpenAIGeneratedFiles(ctx context.Context, msgID string, turn memory.TurnMeta, info Info) error {
	if !openAIGeneratedFilesEnabled() {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(info.Provider), "openai") {
		return nil
	}
	if len(info.ResponseJSON) == 0 {
		return nil
	}
	store, ok := o.client.(apiconv.GeneratedFileClient)
	if !ok {
		return nil
	}
	refs := extractOpenAIGeneratedFiles(info.ResponseJSON)
	if len(refs) == 0 {
		return nil
	}

	input := &gfread.Input{ConversationID: turn.ConversationID, TurnID: turn.TurnID, MessageID: msgID, Has: &gfread.Has{ConversationID: true, TurnID: true, MessageID: true}}
	existing, err := store.GetGeneratedFiles(ctx, input)
	if err != nil {
		return err
	}
	existingByKey := map[string]*gfread.GeneratedFileView{}
	for _, item := range existing {
		if item == nil {
			continue
		}
		key := generatedFileDedupKey(item.Mode, ptrValueString(item.ContainerID), ptrValueString(item.ProviderFileID), ptrValueString(item.Checksum), ptrValueString(item.Filename))
		if key == "" {
			continue
		}
		existingByKey[key] = item
	}

	defaultCopyMode := openAIGeneratedFileCopyMode()
	for _, ref := range refs {
		mode := strings.TrimSpace(ref.Mode)
		if mode == "" {
			mode = "interpreter"
		}
		copyMode := defaultCopyMode
		if mode != "interpreter" {
			copyMode = "eager"
		}
		status := "ready"
		payloadID := ""
		errMsg := ""

		filename := strings.TrimSpace(ref.Filename)
		if filename == "" {
			if mode == "interpreter" && strings.TrimSpace(ref.ProviderFileID) != "" {
				filename = strings.TrimSpace(ref.ProviderFileID)
			} else {
				filename = "generated-file.bin"
			}
		}
		mimeType := strings.TrimSpace(ref.MimeType)
		sizeBytes := ref.SizeBytes
		checksum := strings.TrimSpace(ref.Checksum)

		if len(ref.InlineBody) > 0 {
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			if sizeBytes <= 0 {
				sizeBytes = len(ref.InlineBody)
			}
			if checksum == "" {
				checksum = sha256Hex(ref.InlineBody)
			}
			pid, pErr := o.upsertInlinePayload(ctx, "", "model_response", mimeType, ref.InlineBody)
			if pErr != nil {
				status = "failed"
				errMsg = pErr.Error()
			} else {
				payloadID = pid
			}
		} else if mode == "interpreter" && copyMode == "eager" {
			if strings.TrimSpace(ref.ContainerID) == "" || strings.TrimSpace(ref.ProviderFileID) == "" {
				status = "failed"
				errMsg = "missing openai container_id/file_id for eager copy"
			} else {
				data, contentType, dErr := downloadOpenAIContainerFileContent(ctx, ref.ContainerID, ref.ProviderFileID)
				if dErr != nil {
					status = "failed"
					errMsg = dErr.Error()
				} else {
					if mimeType == "" {
						mimeType = contentType
					}
					if mimeType == "" {
						mimeType = "application/octet-stream"
					}
					sizeBytes = len(data)
					checksum = sha256Hex(data)
					pid, pErr := o.upsertInlinePayload(ctx, "", "model_response", mimeType, data)
					if pErr != nil {
						status = "failed"
						errMsg = pErr.Error()
					} else {
						payloadID = pid
					}
				}
			}
		}

		key := generatedFileDedupKey(mode, ref.ContainerID, ref.ProviderFileID, checksum, filename)
		if existingFile, ok := existingByKey[key]; ok && existingFile != nil {
			upd := apiconv.NewGeneratedFile()
			upd.SetID(existingFile.ID)
			upd.SetCopyMode(copyMode)
			upd.SetStatus(status)
			if payloadID != "" {
				upd.SetPayloadID(payloadID)
			}
			if mimeType != "" {
				upd.SetMimeType(mimeType)
			}
			if sizeBytes > 0 {
				upd.SetSizeBytes(sizeBytes)
			}
			if checksum != "" {
				upd.SetChecksum(checksum)
			}
			if errMsg != "" {
				upd.SetErrorMessage(errMsg)
			}
			if err := store.PatchGeneratedFile(ctx, upd); err != nil {
				return err
			}
			continue
		}

		rec := apiconv.NewGeneratedFile()
		rec.SetID(uuid.NewString())
		rec.SetConversationID(turn.ConversationID)
		if strings.TrimSpace(turn.TurnID) != "" {
			rec.SetTurnID(turn.TurnID)
		}
		if strings.TrimSpace(msgID) != "" {
			rec.SetMessageID(msgID)
		}
		rec.SetProvider("openai")
		rec.SetMode(mode)
		rec.SetCopyMode(copyMode)
		rec.SetStatus(status)
		if strings.TrimSpace(ref.ContainerID) != "" {
			rec.SetContainerID(strings.TrimSpace(ref.ContainerID))
		}
		if strings.TrimSpace(ref.ProviderFileID) != "" {
			rec.SetProviderFileID(strings.TrimSpace(ref.ProviderFileID))
		}
		if filename != "" {
			rec.SetFilename(filename)
		}
		if mimeType != "" {
			rec.SetMimeType(mimeType)
		}
		if sizeBytes > 0 {
			rec.SetSizeBytes(sizeBytes)
		}
		if checksum != "" {
			rec.SetChecksum(checksum)
		}
		if payloadID != "" {
			rec.SetPayloadID(payloadID)
		}
		if errMsg != "" {
			rec.SetErrorMessage(errMsg)
		}
		if err := store.PatchGeneratedFile(ctx, rec); err != nil {
			return err
		}
		existingByKey[key] = &gfread.GeneratedFileView{ID: rec.ID}
	}
	return nil
}

func extractOpenAIGeneratedFiles(raw []byte) []openAIGeneratedFileRef {
	if len(raw) == 0 {
		return nil
	}
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	roots := make([]interface{}, 0, 2)
	if m, ok := payload.(map[string]interface{}); ok {
		if response, ok := m["response"].(map[string]interface{}); ok {
			if output := response["output"]; output != nil {
				roots = append(roots, output)
			}
		}
		if output := m["output"]; output != nil {
			roots = append(roots, output)
		}
		if len(roots) == 0 {
			roots = append(roots, payload)
		}
	} else {
		roots = append(roots, payload)
	}

	candidates := make([]openAIGeneratedFileRef, 0)
	for _, root := range roots {
		walkOpenAIGeneratedFiles(root, "", &candidates)
	}
	if len(candidates) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]openAIGeneratedFileRef, 0, len(candidates))
	for _, item := range candidates {
		key := generatedFileDedupKey(item.Mode, item.ContainerID, item.ProviderFileID, item.Checksum, item.Filename)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func walkOpenAIGeneratedFiles(node interface{}, containerID string, out *[]openAIGeneratedFileRef) {
	switch actual := node.(type) {
	case []interface{}:
		for _, item := range actual {
			walkOpenAIGeneratedFiles(item, containerID, out)
		}
	case map[string]interface{}:
		currContainer := containerID
		if v := stringValue(actual["container_id"]); v != "" {
			currContainer = v
		}

		if fileData := stringValue(actual["file_data"]); fileData != "" {
			body, mimeType := decodeOpenAIFileData(fileData)
			if len(body) > 0 {
				if metaMime := stringValue(actual["mime_type"]); metaMime != "" {
					mimeType = metaMime
				}
				filename := firstNonEmpty(
					stringValue(actual["filename"]),
					stringValue(actual["name"]),
					stringValue(actual["title"]),
				)
				*out = append(*out, openAIGeneratedFileRef{
					Mode:       "inline",
					Filename:   filename,
					MimeType:   mimeType,
					SizeBytes:  len(body),
					Checksum:   sha256Hex(body),
					InlineBody: body,
				})
			}
		}

		providerFileID := stringValue(actual["file_id"])
		if providerFileID == "" && strings.EqualFold(stringValue(actual["type"]), "file") {
			providerFileID = stringValue(actual["id"])
		}
		if providerFileID != "" {
			filename := firstNonEmpty(
				stringValue(actual["filename"]),
				stringValue(actual["name"]),
				stringValue(actual["title"]),
				providerFileID,
			)
			mimeType := stringValue(actual["mime_type"])
			sizeBytes := intValue(actual["size_bytes"])
			if sizeBytes <= 0 {
				sizeBytes = intValue(actual["size"])
			}
			*out = append(*out, openAIGeneratedFileRef{
				Mode:           "interpreter",
				ContainerID:    currContainer,
				ProviderFileID: providerFileID,
				Filename:       filename,
				MimeType:       mimeType,
				SizeBytes:      sizeBytes,
			})
		}

		for _, value := range actual {
			walkOpenAIGeneratedFiles(value, currContainer, out)
		}
	}
}

func decodeOpenAIFileData(raw string) ([]byte, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		commaIdx := strings.Index(raw, ",")
		if commaIdx == -1 {
			return nil, ""
		}
		header := raw[:commaIdx]
		payload := raw[commaIdx+1:]
		mimeType := ""
		if strings.HasPrefix(strings.ToLower(header), "data:") {
			meta := strings.TrimPrefix(header, "data:")
			parts := strings.Split(meta, ";")
			if len(parts) > 0 {
				mimeType = strings.TrimSpace(parts[0])
			}
		}
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(payload)
			if err != nil {
				return nil, ""
			}
		}
		return decoded, mimeType
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil, ""
		}
	}
	return decoded, ""
}

func generatedFileDedupKey(mode, containerID, providerFileID, checksum, filename string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	containerID = strings.TrimSpace(containerID)
	providerFileID = strings.TrimSpace(providerFileID)
	checksum = strings.TrimSpace(strings.ToLower(checksum))
	filename = strings.TrimSpace(strings.ToLower(filename))
	if mode == "interpreter" {
		if providerFileID == "" {
			return ""
		}
		return strings.Join([]string{"interpreter", containerID, providerFileID}, "|")
	}
	if checksum != "" {
		return strings.Join([]string{"inline", checksum, filename}, "|")
	}
	if filename != "" {
		return strings.Join([]string{"inline", filename}, "|")
	}
	return ""
}

func openAIGeneratedFilesEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(envOpenAIGeneratedFileEnabled)))
	switch v {
	case "", "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func openAIGeneratedFileCopyMode() string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(envOpenAIGeneratedFileCopyMode)))
	switch v {
	case "eager":
		return "eager"
	case "lazy":
		return "lazy"
	case "lazy_cache", "lazycache", "cache":
		return "lazy_cache"
	default:
		return "lazy"
	}
}

func downloadOpenAIContainerFileContent(ctx context.Context, containerID, fileID string) ([]byte, string, error) {
	containerID = strings.TrimSpace(containerID)
	fileID = strings.TrimSpace(fileID)
	if containerID == "" || fileID == "" {
		return nil, "", fmt.Errorf("container_id and file_id are required")
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, "", fmt.Errorf("OPENAI_API_KEY is not configured")
	}
	base := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(strings.ToLower(base), "/v1") {
		base += "/v1"
	}
	endpoint := fmt.Sprintf("%s/containers/%s/files/%s/content", base, url.PathEscape(containerID), url.PathEscape(fileID))

	httpClient := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("openai download failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	return body, contentType, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringValue(v interface{}) string {
	switch actual := v.(type) {
	case string:
		return strings.TrimSpace(actual)
	case fmt.Stringer:
		return strings.TrimSpace(actual.String())
	default:
		return ""
	}
}

func intValue(v interface{}) int {
	switch actual := v.(type) {
	case int:
		return actual
	case int32:
		return int(actual)
	case int64:
		return int(actual)
	case float64:
		return int(actual)
	case float32:
		return int(actual)
	case json.Number:
		n, _ := actual.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(actual))
		return n
	default:
		return 0
	}
}

func ptrValueString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
