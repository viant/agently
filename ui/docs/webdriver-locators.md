# Webdriver Locators (Forge / Agently)

## Navigation tree

Agently navigation items render stable `data-testid` attributes.

Examples:
- Top-level Chat group: `nav-chat`
- Nested Chat window: `nav-chat.chat-new`
- Nested History window: `nav-chat.chat-history`

Recommended locators:
- `{"testID":"nav-chat.chat-new"}` (best)
- `{"css":"[data-testid='nav-chat.chat-new']"}` (ok)
- Avoid `{"text":"Chat"}` unless you scope it to a container (multiple “Chat” nodes exist).

## Why

Blueprint Tree renders nested labels and multiple nodes with the same visible text (e.g. group label and leaf label), so text-based locators can match the wrong element.

## Chat view

Forge chat now renders stable `data-testid` markers for automation.

Common locators:
- Root: `{"testID":"chat-root"}`
- Toolbar wrapper: `{"testID":"chat-toolbar"}`
- Toolbar buttons from metadata ids:
  - New conversation: `{"testID":"toolbar-btn-newconv"}`
  - Compact: `{"testID":"toolbar-btn-compact"}`
  - Queue: `{"testID":"toolbar-btn-queue"}`
- Feed: `{"testID":"chat-feed"}`
- Load previous: `{"testID":"chat-feed-load-previous"}`
- Composer:
  - Form: `{"testID":"chat-composer"}`
  - Input: `{"testID":"chat-composer-input"}`
  - Send: `{"testID":"chat-composer-send"}`
  - Abort: `{"testID":"chat-composer-abort"}`
  - Attach: `{"testID":"chat-composer-attach"}`
  - Settings: `{"testID":"chat-composer-settings"}`
  - Mic: `{"testID":"chat-composer-mic"}`
