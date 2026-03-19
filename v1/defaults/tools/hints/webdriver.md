This document is a user-facing hint for MCP webdriver usage in Agently.

## Quick start

- Use `webdriver:*` tools to control a browser session.
- Prefer stable locators (`role`/`name`, `testID`, exact `text`) over brittle CSS.
- Capture a screenshot or DOM when debugging.

## Suggested workflow

1. `webdriverOpen` / `browserOpen` to start a session.
2. `browserGetDOM` / `browserFind` to locate the target element.
3. `browserClick` / `browserFill` / `browserPress` to act.
4. `browserCaptureStart` + run steps + `browserCaptureExport` for debug.

