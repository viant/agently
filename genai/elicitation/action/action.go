package action

import "strings"

const (
    // Actions – per MCP spec
    Accept = "accept"
    Decline = "decline"
    Cancel = "cancel"

    // Statuses – persisted on messages
    StatusAccepted = "accepted"
    StatusRejected = "rejected"
    StatusCancel   = "cancel"
)

// Normalize converts freeform input to canonical MCP action values.
func Normalize(s string) string {
    st := strings.ToLower(strings.TrimSpace(s))
    switch st {
    case "accept", "accepted", "approve", "approved", "yes", "y":
        return Accept
    case "cancel", "canceled", "cancelled":
        return Cancel
    case "decline", "denied", "deny", "reject", "rejected", "no", "n":
        fallthrough
    default:
        return Decline
    }
}

// ToStatus maps an action to persisted message status.
func ToStatus(action string) string {
    switch Normalize(action) {
    case Accept:
        return StatusAccepted
    case Cancel:
        return StatusCancel
    default:
        return StatusRejected
    }
}

// FromStatus maps a message status to MCP action.
func FromStatus(status string) string {
    st := strings.ToLower(strings.TrimSpace(status))
    switch st {
    case StatusAccepted:
        return Accept
    case StatusCancel:
        return Cancel
    default:
        return Decline
    }
}

