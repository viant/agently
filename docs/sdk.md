# SDKs (Go and TypeScript)

This repo ships two client SDKs:

- Go: `client/sdk`
- TypeScript (browser): `ui/src/sdk/agentlyClient.ts`

Both target the same HTTP APIs and event stream endpoints.

## Go SDK

### Install

This SDK is part of the Agently repo; import it directly:

```go
import "github.com/viant/agently/client/sdk"
```

### Create a client

```go
client := sdk.New(
  "http://localhost:8080",
  sdk.WithTimeout(30*time.Second),
  sdk.WithTokenProvider(func(ctx context.Context) (string, error) {
    return os.Getenv("AGENTLY_TOKEN"), nil
  }),
)
```

### Create a conversation and send a message

```go
conv, err := client.CreateConversation(ctx, &sdk.CreateConversationRequest{
  Model: "gpt-4.1",
  Agent: "default",
})
if err != nil { /* handle */ }

msg, err := client.PostMessage(ctx, conv.ID, &sdk.PostMessageRequest{
  Content: "Hello!",
})
if err != nil { /* handle */ }
_ = msg.ID
```

### Stream events (SSE)

```go
events, errs, err := client.StreamEvents(ctx, conv.ID, "", []string{"text", "tool_op", "control"})
if err != nil { /* handle */ }
for {
  select {
  case ev, ok := <-events:
    if !ok { return }
    // ev.Event names include:
    // - interim_message
    // - assistant_message
    // - user_message
    // - tool_call_started|tool_call_completed|tool_call_failed
    // - model_call_started|model_call_completed|model_call_failed
    // - attachment_linked
    // - elicitation
    // - error
    // ev.Message holds MessageView; ev.Content may include deltas or meta
  case err := <-errs:
    if err != nil { /* handle */ }
  }
}
```

### Long-poll events (no SSE)

```go
resp, err := client.PollEvents(ctx, conv.ID, "10", []string{"text"}, 5000*time.Millisecond)
if err != nil { /* handle */ }
for _, ev := range resp.Events {
  _ = ev
}
```

### Elicitation (MCP + LLM)

Elicitations are surfaced as assistant/tool messages with `elicitation_id` set.
You can detect them from the stream or query pending elicitations and then
accept/decline via the callback endpoint.

```go
events, errs, err := client.StreamEvents(ctx, conv.ID, "", []string{"control", "text"})
if err != nil { /* handle */ }
go func() {
  for ev := range events {
    if el := sdk.ElicitationFromEvent(ev); el != nil && sdk.IsElicitationPending(ev.Message) {
      _ = client.ResolveElicitation(ctx, conv.ID, el.ElicitationID, "accept",
        map[string]interface{}{"answer": "yes"}, "")
    }
  }
}()
_ = errs
```

```go
pending, err := client.ListPendingElicitations(ctx, conv.ID)
if err != nil { /* handle */ }
for _, el := range pending {
  _ = client.ResolveElicitation(ctx, conv.ID, el.ElicitationID, "decline", nil, "not now")
}
```

### Attachments

```go
up, err := client.UploadAttachment(ctx, "file.txt", bytes.NewReader([]byte("hello")))
if err != nil { /* handle */ }
_, _ = client.PostMessage(ctx, conv.ID, &sdk.PostMessageRequest{
  Content: "See attached.",
  Attachments: []sdk.UploadedAttachment{
    {Name: up.Name, URI: up.URI, Size: int(up.Size), StagingFolder: up.StagingFolder},
  },
})
```

### Auth helpers

```go
_ = client.AuthProviders(ctx)
_ = client.AuthMe(ctx)
_ = client.AuthLogout(ctx)
```

### OAuth helpers (BFF + OOB)

Interactive (BFF) flow: request an auth URL and open it in a browser.

```go
resp, err := client.AuthOAuthInitiate(ctx)
if err != nil { /* handle */ }
fmt.Println("Open:", resp.AuthURL)
```

Out-of-band (OOB) flow (client-side): perform an OAuth login using local secrets
and install a token provider on the SDK client (auto-refresh when expired).

```go
_, err := client.AuthOOBLogin(ctx,
  "/Users/awitas/.secret/idp_local.enc|blowfish://default",
  "scy://secrets/user/dev|blowfish://default",
  []string{"openid"},
)
if err != nil { /* handle */ }
```

Out-of-band (OOB) flow (server-side BFF): ask the server to perform OOB using a
user credential reference and establish a session cookie.

```go
err := client.AuthOOBSession(ctx,
  "/Users/awitas/.secret/user_cred.enc|blowfish://default",
  []string{"openid"},
)
if err != nil { /* handle */ }
```

## TypeScript SDK

### Create a client

```ts
import { AgentlyClient } from "@/sdk/agentlyClient";

const client = new AgentlyClient({
  baseURL: "http://localhost:8080",
  tokenProvider: () => localStorage.getItem("token"),
  useCookies: true,
  timeoutMs: 30000,
});
```

### Send a message

```ts
const conv = await client.createConversation({ model: "gpt-4.1", agent: "default" });
const msg = await client.postMessage(conv.id, { content: "Hello!" });
```

### Stream events (SSE)

```ts
const es = client.streamEvents(conv.id, {
  include: ["text", "tool_op", "control"],
  onEvent: (ev) => {
    // ev.message is MessageView, ev.content may include deltas/meta
    // ev.event is the SSE name (see list above in Go section)
  },
  onError: (err) => console.error(err),
});
// later: es.close()
```

### Long-poll events

```ts
const resp = await client.pollEvents(conv.id, { since: "10", include: ["text"], waitMs: 5000 });
```

### Upload attachments

```ts
const up = await client.uploadAttachment(file);
await client.postMessage(conv.id, {
  content: "See attached",
  attachments: [{ name: up.name, uri: up.uri, size: up.size, stagingFolder: up.stagingFolder }],
});
```

## Event stream shape

SSE (`GET /v1/api/conversations/{id}/events`) and long-poll return the same envelope:

```json
{
  "seq": 123,
  "time": "2026-01-25T00:00:00Z",
  "conversationId": "c1",
  "message": { "id": "m1", "role": "assistant", "type": "text" },
  "contentType": "application/json",
  "content": { "delta": "partial text" }
}
```

Event names (SSE `event:`):

- interim_message
- assistant_message
- user_message
- tool_call_started
- tool_call_completed
- tool_call_failed
- model_call_started
- model_call_completed
- model_call_failed
- attachment_linked
- elicitation
- error

Long-poll response shape:

```json
{
  "events": [/* envelopes */],
  "since": "123"
}
```

## Resume semantics

- For SSE, use `Last-Event-ID` or `since` with the last `seq`.
- For long-poll, pass `since=<seq>` and read the returned `since` as the new cursor.

## Auth modes

Both SDKs support:

- Bearer token via `Authorization: Bearer <token>`
- Cookie/session (for browser clients, pass `useCookies: true`)

See `docs/auth.md` for server-side configuration.
