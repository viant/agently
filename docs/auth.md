# Authentication for Agently (Unified OAuth + Local)

This document describes how to configure authentication for Agently using a single workspace config (`$AGENTLY_ROOT/config.yaml`), the REST endpoints involved, and how the UI integrates with them.

## Overview

Agently supports two high‑level sign‑in styles that can be enabled independently or together:

- Local (username only): Browser obtains a session cookie via `POST /v1/api/auth/local/login`.
- OAuth/OIDC (unified client):
  - SPA (browser) login: The UI completes Code+PKCE (or provider SDK) and calls APIs with `Authorization: Bearer`. Agently validates the token using JWKS (and optional `iss`/`aud` checks).
  - BFF (server) login: The server starts Code+PKCE using the configured OAuth client, exchanges code on callback, and sets a session cookie.

All configuration lives in `$AGENTLY_ROOT/config.yaml` under `auth`.

## Workspace Configuration

```yaml
auth:
  enabled: true
  cookieName: agently_session
  defaultUsername: devuser            # optional (silent local login on boot)
  ipHashKey: "your-hmac-salt"         # required when enabled (HMAC for ip hashing)
  trustedProxies: ["127.0.0.1/32"]    # X-Forwarded-For trust list

  local:
    enabled: true                     # allow local username login

  oauth:
    mode: mixed                       # bearer | spa | bff | mixed
    name: default
    label: "Sign in"
    client:
      # Public metadata for SPA & Bearer
      discoveryURL: "https://issuer/.well-known/openid-configuration"
      jwksURL: ""                      # optional; if set, used directly
      clientID: "YOUR_CLIENT_ID"       # SPA only (no secret)
      redirectURI: "https://yourapp/callback"
      scopes: ["openid","profile","email"]

      # Optional claim checks (applied after signature verification)
      issuer: "https://issuer"
      audiences: ["your-api-audience"]

      # BFF only (server-initiated; secret config stored in scy)
      # Any viant/afs URL to your confidential OAuth client JSON. Examples:
      #   file:///abs/path/oauth.okta.yaml
      #   s3://bucket/path/oauth.okta.yaml
      #   gs://bucket/path/oauth.okta.yaml
      #   gsecret://projects/<proj>/secrets/<name>/versions/latest
      configURL: "file:///abs/path/oauth.client.json"
```

### Modes

- `bearer`: API accepts/validates `Authorization: Bearer` only (resource server).
- `spa`: UI completes OAuth and sends `Bearer` to the API (no cookie needed).
- `bff`: Server runs OAuth and sets a cookie (no Bearer needed).
- `mixed`: Accept both (cookie OR Bearer).

## Scenario Recipes

Below are copy‑paste YAML examples for the `auth` section you can place inside `$AGENTLY_ROOT/config.yaml`. Each recipe shows only the `auth` block — it can be merged with your existing workspace config.

### 1) Local only (Cookie, silent dev login)

```yaml
auth:
  enabled: true
  cookieName: agently_session
  defaultUsername: devuser
  ipHashKey: dev-hmac-salt
  trustedProxies: ["127.0.0.1/32"]
  local:
    enabled: true
```

Flow: UI boots → `/auth/me` → `/auth/providers` → silent local login with `defaultUsername` → Cookie set → protected APIs OK. The `oauth` section is not required for local‑only silent mode.

### 2) BFF OAuth only (Cookie, server‑initiated OAuth)

```yaml
auth:
  enabled: true
  cookieName: agently_session
  ipHashKey: dev-hmac-salt
  trustedProxies: ["127.0.0.1/32"]
  local:
    enabled: false
  oauth:
    mode: bff
    name: default
    label: "Sign in"
    client:
      # See notes below – use any supported AFS URL (file, s3, gs, gsecret, ...)
      configURL: "file:///abs/path/oauth.okta.yaml"
```

#### Preparing the scy OAuth client config

Server‑side BFF uses `oauth.client.configURL` to load a confidential OAuth client (with secret) using [scy](https://github.com/viant/scy). You can provide the config as a plain file (dev) or store it as an encrypted secret with scy (recommended).

1) Create a confidential client in your IdP

- Grant type: Authorization Code + PKCE.
- Redirect URI (dev): `http://localhost:8080/v1/api/auth/oauth/callback`.
- Note the Authorization URL, Token URL, Client ID, Client Secret, and desired scopes (e.g., `openid profile email`).

2) Write the client config (JSON preferred by scy)

```json
{
  "authURL": "https://<issuer>/oauth2/v1/authorize",
  "tokenURL": "https://<issuer>/oauth2/v1/token",
  "clientID": "<your-client-id>",
  "clientSecret": "<your-client-secret>",
  "redirectURL": "http://localhost:8080/v1/api/auth/oauth/callback",
  "scopes": ["openid", "profile", "email"]
}
```

3a) Dev: reference as a file URL

```yaml
auth:
  oauth:
    mode: bff
    client:
      configURL: file:///absolute/path/to/oauth.client.json
```

3b) Prod/secure: store with scy at an AFS URL (e.g., gsecret://, awssm://, azsecret://, s3://, gs://)

Using scy CLI (see `/Users/awitas/go/src/github.com/viant/scy/cmd/README.md`):

```sh
# Install
go install github.com/viant/scy/cmd/scy@latest

# Option A: store in Google Secret Manager (JSON content)
scy secret put \
  --url gsecret://projects/<proj>/secrets/<name>/versions/latest \
  --in /absolute/path/oauth.client.json \
  --key raw:<32-byte-hex-or-string>   # or configure KMS per scy docs

# Option B: upload to object storage (S3/GS)
scy secret put --url s3://bucket/path/oauth.client.json --in /absolute/path/oauth.client.json
scy secret put --url gs://bucket/path/oauth.client.json --in /absolute/path/oauth.client.json
```

Supported URLs (see https://github.com/viant/afsc for the latest list):

- gsecret://projects/<project>/secrets/<name>/versions/latest (Google Secret Manager)
- awssm://<region>/<name> (AWS Secrets Manager)
- azsecret://<vault-name>/<secret-name> (Azure Key Vault)
- s3://bucket/path/file.json, gs://bucket/path/file.json
- file:///abs/path/file.json

Then reference it in Agently (pick the same URL you used above):

```yaml
auth:
  enabled: true
  cookieName: agently_session
  ipHashKey: dev-hmac-salt
  oauth:
    mode: bff
    client:
      configURL: gcp://secretmanager/projects/<proj>/secrets/<name>/versions/latest
    # Optional extra claim checks used during BFF callback verification:
    # client.issuer: https://<tenant>/oauth2/default
    # client.audiences: ["api://agently"]
```

Troubleshooting:

- Callback mismatch: ensure `redirectURL` in your client YAML matches the IdP app exactly.
- Cookie not set: use the same hostname for UI and API in dev (`localhost`), ensure UI fetches with `credentials: include`.
- scy key/secret: provide `SCY_KEY` or `--key`; confirm the AFS URL matches what you used with `scy secret put`.




Flow: UI clicks “Sign in” → `POST /auth/oauth/initiate` → popup → `/auth/oauth/callback` → Cookie set → protected APIs OK. ID token is verified and `iss`/`aud` checks applied when configured.

## MCP Clients (cookie‑first with optional bearer)

When Agently connects to MCP providers (streamable or SSE), the HTTP clients use an auth RoundTripper configured for BFF cookie‑first behavior while allowing bearer tokens:

- Cookies: always attached (from a shared Jar), including during internal retries and metadata fetches.
- Bearer: attached from request context when present and allowed (default allowed origins: ["*"]). Rejected tokens are cached for a TTL to avoid reuse loops.
- 401/419: the transport performs a single BFF exchange (or Protected Resource Metadata flow) and replays the request.

Factory (used by Agently):

```go
rt, _ := integrate.NewAuthRoundTripper(sharedJar, http.DefaultTransport, 10*time.Minute)
client := mcpclient.New("agently", version, transport)
integrate.NewClientWithAuthInterceptor(client, rt)
```

Session resume (recommended for reconnects):

- Streamable (NDJSON):

```go
sc, _ := streamcli.New(ctx, mcpURL,
    streamcli.WithSessionID(savedID),
    streamcli.WithSessionHeaderName("Mcp-Session-Id"),
)
```

- SSE (HTTP streaming):

```go
ssec, _ := sse.New(ctx, streamURL,
    sse.WithSessionID(savedID),
    sse.WithStreamSessionParamName("Mcp-Session-Id"),
)
```

Persist the session id you obtain during the initial handshake and pass it back on restart for smooth reconnects.

### 3) SPA OIDC only (Bearer, browser‑initiated OAuth)


minimal:

```yaml
auth:
  enabled: true
  cookieName: agently_session
  ipHashKey: dev-hmac-salt
  oauth:
    mode: bff
    client:
      configURL: gcp://secretmanager/projects/<proj>/secrets/<name>/versions/latest
      redirectURI: "http://localhost:5173/callback"
```


/sdk/8473/

```yaml
auth:
  enabled: true
  cookieName: agently_session
  ipHashKey: dev-hmac-salt
  trustedProxies: ["127.0.0.1/32"]
  local:
    enabled: false
  oauth:
    mode: spa
    name: default
    label: "Sign in"
    client:
      discoveryURL: "https://issuer/.well-known/openid-configuration"
      clientID: "YOUR_CLIENT_ID"
      scopes: ["openid", "profile", "email"]
      issuer: "https://issuer"
      audiences: ["your-api-audience"]
```

Flow: UI runs OAuth Code+PKCE (provider SDK) → obtains ID token → calls APIs with `Authorization: Bearer` → middleware validates signature via JWKS (resolved from discovery) and applies `iss`/`aud` checks.

### 4) Mixed (Cookie or Bearer accepted)

```yaml
auth:
  enabled: true
  cookieName: agently_session
  ipHashKey: dev-hmac-salt
  trustedProxies: ["127.0.0.1/32"]
  local:
    enabled: true
  oauth:
    mode: mixed
    name: default
    label: "Sign in"
    client:
      configURL: "gsecret://projects/<proj>/secrets/<name>/versions/latest"
      discoveryURL: "https://issuer/.well-known/openid-configuration"
      jwksURL: ""
      clientID: "YOUR_CLIENT_ID"
      redirectURI: "http://localhost:5173/callback"
      scopes: ["openid", "profile", "email"]
      issuer: "https://issuer"
      audiences: ["your-api-audience"]
```

Flow: UI may choose either BFF (cookie) or SPA (Bearer). Middleware accepts both.


## REST Endpoints

### Auth session

- `GET  /v1/api/auth/me`          → current user profile (cookie or Bearer)
- `POST /v1/api/auth/logout`      → clears session cookie

### Providers discovery (UI boot)

- `GET  /v1/api/auth/providers`   → list configured providers
  - `local` (with `defaultUsername` when configured)
  - `bff`   (when `oauth.client.configURL` is set)
  - `oidc`  (SPA/Bearer) with public client metadata: `clientID`, `discoveryURL`, `redirectURI`, `scopes`

### Local login

- `POST /v1/api/auth/local/login` `{ name }` → sets cookie; upserts user; stores `hash_ip`.

### BFF OAuth (server‑initiated)

- `POST /v1/api/auth/oauth/initiate`   → `{ authURL }` (PKCE + encrypted state)
- `GET/POST /v1/api/auth/oauth/callback?code&state`
  - decrypts state → code_verifier
  - exchanges code → token
  - verifies ID token signature via JWKS and checks `iss`/`aud` when configured
  - upserts user (provider: `oauth`), sets session cookie, stores `hash_ip`
  - returns tiny HTML that posts a message to `window.opener` and closes

### Protection (middleware)

When `auth.enabled`:
- Protects `/v1/api/*` and `/v1/workspace/*` except `/v1/api/auth/*` and `OPTIONS`.
- Accepts:
  - Bearer (when `oauth.mode` includes `bearer|spa|mixed`) — validated using JWKS from `oauth.client.jwksURL` or `discoveryURL` (resolved to `jwks_uri`).
  - Cookie (when `oauth.mode` includes `bff|mixed` or `local.enabled` is true).
- Optional claim checks:
  - `issuer`: compared to `iss`
  - `audiences`: at least one must match `aud`

## UI Integration

1) On boot, the UI calls `/v1/api/auth/me`. If 401:
   - Calls `/v1/api/auth/providers`.
   - If `local.defaultUsername` is present → `POST /v1/api/auth/local/login` then `GET /me`.
   - If provider `bff` present → call `POST /v1/api/auth/oauth/initiate`, open popup and wait for a postMessage to close.
   - If provider `oidc` present → initialize your OIDC SPA client using `clientID`, `discoveryURL`, `redirectURI`, `scopes`; after obtaining an ID token, call APIs with `Authorization: Bearer`.

2) Logout → `POST /v1/api/auth/logout` and clear any SPA tokens.

The UI AuthProvider in this repo already implements these strategies and exposes actions (`loginLocal`, `loginBFF`, `loginSPAWithToken`, `logout`).

## Privacy and Security Notes

- The server stores only a salted hash of the client IP (`hash_ip`), never the raw IP.
- No secrets are exposed to the UI:
  - SPA uses only `clientID` and discovery metadata.
  - BFF uses `oauth.client.configURL` (managed by `scy`) on the server.
- Bearer validation uses JWKS and optional `iss`/`aud` checks per config.
- For production, prefer HTTPS everywhere and set Secure cookies via a TLS‑aware reverse proxy.
