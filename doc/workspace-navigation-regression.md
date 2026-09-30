# Workspace navigation regression investigation

## Evidence

- Before Forge `49bb0bd`, `ui.window.open` returned after `addWindow`.
- `49bb0bd` (September 11) added `waitForWorkspaceReady` to navigation. Readiness waits for metadata and permission application. Core can then additionally wait for an eager datasource refresh.
- Conversation `052b9fba-419d-48e8-b9e1-a818d8df272d`, first request September 23: command delivered immediately at 06:50:17; open timed out at 06:50:32; the initiating browser subsequently displayed authorized advertiser rows. The persisted failed tool result did not provide a portable workspace reference.
- A second browser displayed only the failed response, while the initiating browser retained its window. This is insufficient conversation persistence.
- Core `fc8b251a` (September 22) scopes window resources. Loading the real Steward Advertiser List for default, web, phone, tablet, and Android targets resolves **10 datasources** in each case. This scoping must remain enabled.
- The deep-link bootstrap could render canonical chat rows and then skip workspace restoration when the conversation form ID was still empty. That race was repaired in `8e7df184`.

## Intended contract

1. The browser acknowledges successful window creation promptly. Its descriptor can remain `opening`; it must not claim protected content is ready.
2. Authorization and datasource loading remain renderer-owned and must finish before protected content is displayed.
3. A successful navigation result is stored with the server conversation and remains a workspace reference while content loads. Reload and another browser restore from this record.
4. Browser local/session storage must not persist workspace records. In-memory state is only a rendering cache.
5. Each loaded browser document has an independent UI transport ID. Duplicated session storage must not give two tabs the same command queue.
6. Explicit `refreshOnOpen: true` retains its wait-for-readiness contract. Ordinary navigation does not eagerly refresh pending content through a second backend command.

## Verification limits

Unit and integration checks cover pending navigation acknowledgement, canonical references, resource scoping, and restoration. A full acceptance run must additionally demonstrate prompt UI opening, populated authorized content, reopening in a separate browser, and chat navigation. Screenshots of an already-open workspace alone do not establish command latency or cross-window persistence. Do not rewrite earlier failed tool calls as successes.
