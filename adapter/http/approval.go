package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/viant/agently/genai/conversation"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/memory"

	"github.com/viant/fluxor/service/approval"
)

// startApprovalBridge launches a background goroutine that converts Fluxor
// approval events into chat messages (role=="policyapproval") and vice versa.
// It mirrors the behaviour of elicitation and user-interaction bridging so that
// web users can approve or reject tool executions through the existing chat UI.
// StartApprovalBridge launches the bridge.  It is exported so that higher-level
// adapters (e.g. the HTTP router) can hook it up during initialisation.
func StartApprovalBridge(ctx context.Context, exec *execsvc.Service, mgr *conversation.Manager) {
	if exec == nil || mgr == nil {
		return
	}

	go func() {
		// Wait for approval service to become ready (executor boot is async)
		var svc approval.Service
		for svc == nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			svc = exec.ApprovalService()
			if svc == nil {
				time.Sleep(20 * time.Millisecond)
			}
		}

		// Helper to unmarshal args RawMessage -> map[string]interface{}
		parseArgs := func(raw json.RawMessage) map[string]interface{} {
			if len(raw) == 0 {
				return nil
			}
			var out map[string]interface{}
			_ = json.Unmarshal(raw, &out)
			return out
		}

		// Map to remember which conversation owns a given request ID so that
		// we can quickly update the corresponding message on decision.
		id2conv := make(map[string]string)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := svc.Queue().Consume(ctx)
			if err != nil {
				continue // retry unless ctx is cancelled
			}

			evt := msg.T()
			if evt == nil {
				_ = msg.Ack()
				continue
			}

			switch evt.Topic {
			case approval.TopicRequestCreated, approval.LegacyTopicRequestNew:
				req, ok := evt.Data.(*approval.Request)
				if !ok || req == nil {
					_ = msg.Ack()
					continue
				}

				// Attempt to determine conversation ID.
				convID := ""
				if cidRaw, ok := req.Meta["conversationId"]; ok {
					if s, ok2 := cidRaw.(string); ok2 {
						convID = s
					}
				}

				if convID == "" {
					// As a fallback use ProcessID when it looks like a UUID.
					if isUUID(req.ProcessID) {
						convID = req.ProcessID
					}
				}

				if convID == "" {
					// As a fallback pick the latest active conversation so that the
					// user still receives the approval prompt even if we cannot
					// attribute it perfectly.
					if msg, err := mgr.History().LatestMessage(ctx); err == nil && msg != nil {
						convID = msg.ConversationID
					}
				}

				if convID == "" {
					// Ultimately still unknown – drop the prompt.
					_ = msg.Ack()
					continue
				}

				// Determine parentId so the UI poll (parentId=…) sees the prompt.
				parentID := ""
				if lastMsg, err := mgr.History().LatestMessage(ctx); err == nil && lastMsg != nil {
					parentID = lastMsg.ID
					if lastMsg.ParentID != "" {
						parentID = lastMsg.ParentID
					}
				}
				m := memory.Message{
					ID:             req.ID,
					ParentID:       parentID,
					ConversationID: convID,
					Role:           "policyapproval",
					Status:         "open",
					PolicyApproval: &memory.PolicyApproval{
						Tool:   req.Action,
						Args:   parseArgs(req.Args),
						Reason: "",
					},
					// Relative callback path (the Forge UI prefixes /v1/api automatically).
					CallbackURL: "approval/" + req.ID,
				}

				if err := mgr.History().AddMessage(ctx, m); err != nil {
					log.Printf("approval bridge add message error: %v", err)
				} else {
					id2conv[req.ID] = convID
				}

			case approval.TopicDecisionCreated, approval.LegacyTopicDecisionNew:
				dec, ok := evt.Data.(*approval.Decision)
				if !ok || dec == nil {
					_ = msg.Ack()
					continue
				}

				convID := id2conv[dec.ID]
				if convID == "" {
					// Fallback: iterate conversations to find one that has message with that ID.
					convs, _ := mgr.List(ctx)
					for _, cid := range convs {
						msgs, _ := mgr.Messages(ctx, cid, "")
						for _, mm := range msgs {
							if mm.ID == dec.ID {
								convID = cid
								id2conv[dec.ID] = cid
								break
							}
						}
						if convID != "" {
							break
						}
					}
				}

				fmt.Println("convID:", convID)
				if convID != "" {
					newStatus := "declined"
					if dec.Approved {
						newStatus = "done"
					}
					_ = mgr.History().UpdateMessage(ctx, dec.ID, func(m *memory.Message) {
						m.Status = newStatus
						if m.PolicyApproval != nil {
							m.PolicyApproval.Reason = dec.Reason
						}
					})
				}

			}

			_ = msg.Ack()
		}
	}()
}

// isUUID performs a minimal check whether the supplied string resembles a UUID.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	parts := strings.Split(s, "-")
	return len(parts) == 5
}
