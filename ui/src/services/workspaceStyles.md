# Workspace appearance integration

`WorkspaceStyleManager` owns one document's CSS/catalog snapshot and selection. `WorkspaceStyleProvider` reference-counts that document manager and passes selection to Forge. `AppWorkspaceStyles` handles fresh metadata, SDK-authenticated asset reads, auth cleanup, and the same-origin Forge iframe bridge. The preview host uses the same provider with its own metadata response.

The bridge checks the actual iframe/parent source and exact origin, then verifies workspace and catalog revision before applying a known theme/mode. A trusted parent's identity/revision change triggers a fresh local metadata read; it does not authorize applying foreign data. The bridge carries no CSS or asset URLs. A valid parent selection is retained during a catalog refresh to avoid reverting briefly to the guest default.

Hosts with nonce-based style policy may supply `nonce` to the provider or expose it in `meta[name="agently-style-nonce"]` or an existing nonce-bearing script. A rejected stylesheet retains the last valid appearance and produces a diagnostic.

Focused tests (from `agently/ui`):

```sh
APPSERVER_URL=http://127.0.0.1:8080 npm test -- --run src/services/workspaceStyles.test.js src/services/workspaceThemeBridge.test.js
```

`workspace-theme-proof.html` is a development proof entry, excluded from the normal production build. It deliberately disables parent preference storage and embeds the actual `/mcp-ui/forge-window` route. Use a backend workspace with a `style-proof` window and named themes. To produce both proof and application pages:

```sh
APPSERVER_URL=http://127.0.0.1:8080 npm exec -- vite build --config vite.theme-proof.config.js
```

Serve `theme-proof-dist` with the usual backend APIs and SPA routing, including the proof HTML itself. Enter a value in the iframe, switch the parent theme/mode, edit a theme token, and reload workspace appearance. The iframe must update without navigation or losing the value, despite disabled parent preference storage.
