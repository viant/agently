# Report chat layout in Design System 2.0

Reports opened from a conversation reuse the shared Chat options menu: Split on left, Split on right, and Overlay. The initial split places chat on the left. The existing conversation, report, and draft remain mounted through placement changes; native report capability restrictions are preserved.

In document split view, MenuBar owns the chat-header slot, Root passes its native mount to ConversationWorkspaceSurface, and that surface places the same ChatPlacementMenu beside the Chat title. The transcript has no second header. The shell header follows its conversation pane when chat moves right. A standalone host without a shell slot retains its own controls.

Overlay has its own title and menu. The existing floating panel keeps its 560px maximum width, with a 480px height cap and content-area bounds. The transcript scrolls separately from the native composer. Narrow hosts retain the existing automatic overlay and unavailable split choices.

Appearance belongs to AI Studio style://global/workspace, scoped to DS2. Reuse the existing surface, border, radius, typography, spacing, and overlay-shadow roles. The compound document composer has one outer field and a full-width transparent textarea; standalone Textarea recipes exclude it.

Keep native menu semantics and focus handling. Pointer interaction adds no focus outline; keyboard focus uses the shared semantic inset indicator, and the composer has one outer indicator. Escape dismisses the menu. Existing Baseline and legacy controls remain unchanged.
