Title: MCP Auth Reuse — Staged Rollout and Operations

Objectives
- Roll out bearer-first MCP auth reuse safely, minimize re-auth, maintain strict security.

Feature Flags and Controls
- Global default: default.mcp.reuseAuthorizer (optional; unset = disabled).
- Global mode: default.mcp.reuseAuthorizerMode (optional; defaults to bearer_first when needed).
- Env overrides: MCP_REUSE_AUTHORIZER, MCP_REUSE_AUTHORIZER_MODE.
- Provider overrides: mcp.providers[].auth.reuseAuthorizer/mode.
- Client runtime: MCPReuseAuthorizer *bool, MCPReuseAuthorizerMode *string.
- Per-request: WithReuseAuthorizer(ctx,bool), WithReuseAuthorizerMode(ctx,mode).

Prerequisites
- Broker endpoints deployed (exchange/refresh/peek) with CSRF protection.
- Origin allowlists configured for MCP service endpoints.
- Audience allowlists configured per provider.
- Refresh persistence: OS keychain or encrypted file store (AES‑GCM) with secured keys.

Staged Rollout Plan
- Stage 1 (Staging):
  - Enable defaults: reuse=true, mode=bearer_first.
  - Validate 200s with bearer-only; ensure no unexpected re-auth prompts.
  - Observe metrics: auth_header_bearer, token_store_hit, broker refresh/exchange counts.
- Stage 2 (Canary 10%):
  - Enable for a subset of MCP providers or users.
  - Monitor 401 retry success, error rates, and latency impact.
  - SLO gates: retry success > 99%, 401 rates within baseline + small delta.
- Stage 3 (50% → 100%):
  - Ramp if SLOs met; continue monitoring metrics and logs (no secrets).
  - Validate broker capacity and refresh/exchange patterns (preemptive TTL prevents spikes).

Rollback Strategy
- Immediate disable via env: MCP_REUSE_AUTHORIZER=false.
- Provider-specific disable via config: auth.reuseAuthorizer=false.
- Switch mode to cookie_first temporarily if bearer-first issues observed.
- Revert to previous build if structural issues.

Operational Runbook
- Observability: track auth_retry_401/success, auth_token_broker_refresh/exchange, token_store_hit.
- Alerts: high 401 rates, broker errors, refresh failures.
- Security checks: ensure HTTPS-only (AllowInsecure=false), origin/audience allowlists enforced.
- Cleanup: use store.Logout(key) on user logout to zeroize tokens.

Troubleshooting
- 401 loops: verify audience mapping, authority match, and allowlists. Check broker exchange claims.
- No header emitted: check reuse/mode resolution precedence and allowlists.
- Cross-site issues: verify broker endpoints and CORS/credentials; bearer-first avoids MCP cookie dependency.

Appendix: Example Config Snippet
- default:
    mcp:
      reuseAuthorizer: true
      reuseAuthorizerMode: bearer_first
      minTTL:
        access: 30m
        id: 10m
      storage:
        access: memory
        id: memory
        refresh: encrypted
- mcp:
    providers:
      - name: tools-viant
        auth:
          reuseAuthorizer: true
          reuseAuthorizerMode: bearer_first
          authority: https://idp.example.com/realms/acme
          audience: mcp:tools
