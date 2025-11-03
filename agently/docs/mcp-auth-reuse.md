Title: MCP Auth Reuse (Bearer-First) — Design and Configuration

Overview
- Purpose: Reuse the application’s BFF OAuth session to authenticate MCP providers with bearer-first semantics to minimize re-auth prompts.
- Scope: Authority mapping, per-request overrides, token storage policy, broker-backed refresh/exchange, retry and observability.

Key Concepts
- Authority mapping: Reuse only when app and MCP share the same issuer or origin (scheme://host[:port]).
- Bearer-first: Send Authorization: Bearer on the first MCP request; no cookie dependence.
- Broker: Use cookie-authenticated BFF endpoints to exchange/refresh tokens when needed.
- Storage policy: access/id in memory; refresh encrypted-persistent by default; zeroization on logout.
- Precedence: context > client > CLI > env > provider > global > built-in.
  - Built-in default: reuse is disabled unless explicitly enabled.

Configuration
- Global defaults (YAML, preferred):
  - default:
      mcp:
        reuseAuthorizer: true               # optional (pointer); omit to keep global disabled
        reuseAuthorizerMode: bearer_first   # optional (pointer)
      minTTL:
        access: 30m
        id: 10m
      storage:
        access: memory
        id: memory
        refresh: encrypted
- Per provider (YAML):
  - mcp:
    providers:
      - name: tools-viant
        auth:
          reuseAuthorizer: true
          reuseAuthorizerMode: bearer_first
          authority: https://idp.example.com/realms/acme
          audience: mcp:tools
          minTTL:
            access: 30m
            id: 10m
          storage:
            access: memory
            id: memory
            refresh: encrypted

Runtime Overrides
- Env:
  - MCP_REUSE_AUTHORIZER=true|false
  - MCP_REUSE_AUTHORIZER_MODE=bearer_first|cookie_first
- Client-level (runtime):
  - MCPReuseAuthorizer *bool (nil → fallback to config)
  - MCPReuseAuthorizerMode *string (nil → fallback to config)
- Per-request:
  - WithReuseAuthorizer(ctx, bool)
  - WithReuseAuthorizerMode(ctx, bearer_first|cookie_first)

Security Defaults
- HTTPS-only header emission (AllowInsecure=false recommended).
- Origin allowlist and SameAuthAuthority required.
- Audience allowlist recommended per provider.
- No token logging; zeroization on delete; refresh encrypted-at-rest.

Integration Points
- Token store/backends: tokens.Store + RefreshStore (memory or encrypted file; keychain recommended in production).
- Resolver: broker-backed EnsureAccess/EnsureAccessAndID with singleflight, MinTTL.
- MCP HTTP:
  - BuildAuthHeader: applies bearer-first when reuse enabled and allowed.
  - WithBearerFirstRetry: one 401/419-triggered refresh + retry.

Observability
- Tracer events: auth_header_bearer_first, token_store_hit, token_broker_refresh/exchange, auth_retry_401, auth_retry_success, and guard denials.
- Metrics counters: auth_header_bearer, auth_token_store_hit, auth_token_broker_refresh/exchange, auth_retry_401/success.

Example Flow
1) Resolve reuse/mode from config/env/CLI/client/context.
2) Before MCP call, BuildAuthHeader ensures a valid bearer token via resolver.
3) On 401/419, WithBearerFirstRetry invalidates access, refreshes token via broker, retries once.
