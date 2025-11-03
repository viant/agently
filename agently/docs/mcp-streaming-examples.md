Title: MCP Streaming (WebSocket + SSE) — Bearer-First Auth Examples

Bootstrap (Provider-wide)
- Build ProviderClient with auth for both WS and SSE:
  - pc, err := integrate.Bootstrap(integrate.BootstrapConfig{
      Name: "agently", Version: version,
      Transport: wsTransport, Reconnect: reconnectFn,
      AppAuthority: appAuthority, MCPAuthority: providerAuthority,
      CookieJar: sharedBFFJar, BaseRoundTripper: http.DefaultTransport, RejectCacheTTL: 10*time.Minute,
      Resolver: resolver, TokenKey: tokenKey,
    })

WebSocket (Streamable)
- Build auth transport and attach Authorizer:
  - rt, _ := integrate.NewAuthRoundTripper(sharedJar, http.DefaultTransport, 10*time.Minute)
  - client := mcpclient.New("agently", version, wsTransport)
  - integrate.NewClientWithAuthInterceptor(client, rt)
- Token function from resolver:
  - tokenFn := integrate.TokenFnFromResolver(resolver, key)
- Subscribe (bearer-first + session resume):
  - res, err := integrate.SubscribeWithAuth(ctx, pc.Client, params, pc.TokenFn)
  - // If you persist session ids, you can reconnect without a fresh handshake:
  - saved := loadSessionID() // string
  - sc, _ := streamcli.New(ctx, mcpURL,
        streamcli.WithSessionID(saved),                // resume
        streamcli.WithSessionHeaderName("Mcp-Session-Id"),
    )
- Reconnect on auth-related close (one attempt):
  - err := integrate.RunWithAuthReconnect(ctx, pc.TokenFn, clientReconnect, func(ctx context.Context, token string) (stop func(), errCh <-chan error, err error) {
      // Add token via WithAuthToken for any initial RPCs
      _, err = pc.Client.Subscribe(ctx, params, mcpclient.WithAuthToken(token))
      // Start your stream reading; return stop and an err channel here
      return stop, errs, err
    })

SSE (HTTP Streaming)
- Build http.Client with auth RoundTripper: (already provided by bootstrap)
  - hc := pc.SSEClient
- Token function from resolver:
  - tokenFn := pc.TokenFn
- Open with bearer-first and single 401 retry:
  - open := func(ctx context.Context, token string) (io.ReadCloser, *http.Response, error) {
      return integrate.SSEOpenHTTP(ctx, hc, url, token, nil)
    }
  - rc, resp, err := integrate.OpenSSEWithAuth(ctx, open, pc.TokenFn)
- Reconnect mid-stream once:
  - Read from rc; on first unexpected close, reacquire token and call OpenSSEWithAuth again.
 - Resume session id on restart:
   - saved := loadSessionID()
   - sseClient, _ := sse.New(ctx, streamURL,
         sse.WithSessionID(saved),                      // resume
         sse.WithStreamSessionParamName("Mcp-Session-Id"),
     )

Guards (apply before sending Authorization)
- SameAuthAuthority(appAuthority, providerAuthority)
- Origin allowlist includes MCP server origin
- Audience allowlist includes provider audience
- HTTPS-only (do not send Authorization on http)

Notes
- Cookie-first with optional bearer: the auth RoundTripper always sends cookies; it adds Authorization from context when present and allowed. Defaults are permissive for origins.
- WithAuthToken injects Authorization on HTTP transports and _meta.authorization.token on stdio transports.
- The Auth RoundTripper retries handshake on 401 using BFF exchange or OAuth, then returns the retried result. Rejected bearers are cached for a TTL to avoid loops.
- RunWithAuthReconnect covers mid-stream reconnect setup; you provide the runner that starts/monitors your stream and returns an error channel.
