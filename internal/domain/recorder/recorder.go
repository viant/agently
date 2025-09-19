package recorder

import (
	"context"
	"encoding/json"
	"os"

	convcli "github.com/viant/agently/client/conversation"
	convimpl "github.com/viant/agently/internal/service/conversation"
)

type Mode string

const (
	ModeOff    Mode = "off"
	ModeShadow Mode = "shadow"
	ModeFull   Mode = "full"
)

// Enablement exposes a simple toggle to guard writes based on mode.
type Enablement interface {
	Enabled() bool
}

// MessageRecorder persists messages.
// MessageRecorder removed. Use conversation client in services.

// Tool call persistence moved to client/conversation. Recorder no longer handles it.
// Model call persistence moved to client usage; recorder no longer exposes it.

// Recorder is the unified surface that composes the smaller responsibilities.
// Downstream code can depend on individual sub-interfaces to reduce coupling
// and enable plugging alternative implementations (e.g. history DAO, exec traces).
type Recorder interface{}

// Writer is kept as a backward-compatible alias for Recorder.
// Deprecated: prefer depending on specific sub-interfaces or Recorder.
type Writer = Recorder

var _ Recorder = (*Store)(nil)

type Store struct {
	mode   Mode
	client convcli.Client
}

// RecordMessage removed. Services write messages via conversation client directly.

// Deprecated: StartTurn/UpdateTurn removed. Use conversation client PatchTurn directly.

// Tool call start/finish methods removed. Tools are persisted via conversation client by the caller.

// Deprecated RecordModelCall removed; use StartModelCall and FinishModelCall instead.

// Model call methods removed; use conversation client directly where needed.

func toJSONBytes(v interface{}) []byte {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []byte:
		return t
	case json.RawMessage:
		return []byte(t)
	default:
		b, _ := json.Marshal(v)
		return b
	}
}

// New builds a store-backed Writer using in-memory DAO backends by default.
// When AGENTLY_DOMAIN_MODE is "off" it returns a disabled writer.
func New(ctx context.Context) Writer {
	mode := Mode(os.Getenv("AGENTLY_DOMAIN_MODE"))
	if mode == "" {
		// Default to shadow writes when not explicitly configured so
		// v1 endpoints can persist via DAO-backed store out of the box.
		mode = ModeShadow
	}
	if mode == ModeOff {
		return &Store{mode: ModeOff}
	}
	// Build conversation client from environment
	dao, err := convimpl.NewDatlyServiceFromEnv(ctx)
	if err != nil {
		return &Store{mode: ModeOff}
	}
	client, err := convimpl.New(ctx, dao)
	if err != nil {
		return &Store{mode: ModeOff}
	}
	return &Store{mode: mode, client: client}
}

func strp(s string) *string {
	if s != "" {
		return &s
	}
	return nil
}
