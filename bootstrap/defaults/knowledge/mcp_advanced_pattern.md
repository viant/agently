# Advanced MCP Patterns: OOB, Locking, Namespaces

This document captures production patterns for robust MCP servers.

## 1) Namespace Isolation
Goal: isolate state per user/session/tenant.

Pattern:
- resolve namespace from context
- bind service instances by namespace
- cache per-namespace services safely

Benefits:
- no token/state leakage across users
- cleaner multi-tenant behavior under concurrency

## 2) OOB Authorization Routing
Goal: complete auth in browser while preserving request context.

Pattern:
- create pending auth IDs
- map callback requests back to namespace/session using pending ID
- resume blocked operations after auth completion

Requirements:
- pending IDs must expire
- callbacks must be tamper-resistant

Concrete namespace + pending model:
```go
type PendingAuth struct {
	ID        string
	Namespace string
	Alias     string
	Domain    string
	ExpiresAt time.Time
}

var pendingByID = map[string]PendingAuth{}
```

Create pending in tool path:
```go
func startPending(ns, alias, domain string) (pendingID string, oobURL string) {
	pid := uuid.NewString()
	pendingByID[pid] = PendingAuth{
		ID: pid, Namespace: ns, Alias: alias, Domain: domain, ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	return pid, "http://127.0.0.1:8081/oob/start?pending=" + url.QueryEscape(pid)
}
```

Resolve namespace in callback:
```go
func namespaceFromPending(pid string) (string, bool) {
	p, ok := pendingByID[pid]
	if !ok || time.Now().After(p.ExpiresAt) {
		return "", false
	}
	return p.Namespace, true
}
```

## 3) Credential Acquisition Locking
Goal: prevent auth storms from parallel requests.

Pattern:
- key lock by namespace + credential scope
- first request acquires token (leader)
- followers wait on completion channel
- all proceed after success/failure signal

Requirements:
- waiter timeout and cancellation support
- always release lock on error paths

## 4) Prompt Dedupe and Cooldown
Goal: avoid repeated elicitation/auth prompts.

Pattern:
- track recent prompt keys by scope
- suppress duplicates during cooldown window
- allow new prompt after state change or expiry

## 5) Waiter Notification Model
Goal: wake blocked requests precisely.

Pattern:
- targeted notify by scope key
- optional broader notify when shared credential updated
- timeout-safe waiting primitives

## 6) State Partitioning and Locks
Do not use one global mutex for everything.

Use separate protected maps for:
- token state
- pending auth records
- prompt dedupe windows
- permission or visibility caches

This reduces contention and deadlock risk.

## 7) Capability-Aware Error Behavior
For optional capabilities (elicitation/sampling):
- check client support first
- return explicit `isError` results if unavailable
- avoid hidden retries and silent hangs

## 8) Bridge Interoperability
When bridging stdio clients to HTTP servers:
- normalize capability advertisement
- ensure request/response envelopes remain protocol-correct
- validate both direct and bridged execution paths

## 9) Practical Adoption Order
1. namespace isolation
2. OOB pending map and callback routing
3. singleflight-style credential lock
4. prompt dedupe/cooldown
5. bridge + compatibility validation

## 10) OOB HTML Embedded Separation Pattern
Keep the OOB web layer separate from MCP handlers:
- `/oob/start`: minimal HTML page with consent/login button
- `/oob/callback`: token exchange + waiter notification
- MCP tool handler only emits URL and waits with timeout

`/oob/start` embedded HTML example:
```go
func oobStart(w http.ResponseWriter, r *http.Request) {
	pid := r.URL.Query().Get("pending")
	html := `<!doctype html><html><body>
<h3>GitHub Authorization</h3>
<p>Pending: ` + template.HTMLEscapeString(pid) + `</p>
<form action="/oob/callback" method="GET">
  <input type="hidden" name="pending" value="` + template.HTMLEscapeString(pid) + `"/>
  <input type="text" name="token" placeholder="GitHub token"/>
  <button type="submit">Complete</button>
</form>
</body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
```

`/oob/callback` separation (state update only):
```go
func oobCallback(w http.ResponseWriter, r *http.Request) {
	pid := r.URL.Query().Get("pending")
	token := r.URL.Query().Get("token")

	ns, ok := namespaceFromPending(pid)
	if !ok {
		http.Error(w, "invalid/expired pending", http.StatusBadRequest)
		return
	}

	saveToken(ns, token) // namespace-scoped token store
	notifyNamespace(ns)  // wake waiters blocked in tool calls
	delete(pendingByID, pid)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OOB auth complete"))
}
```

Tool-side fire-and-wait:
```go
ns := namespace.FromContext(ctx)
_, oobURL := startPending(ns, alias, domain)
_, _ = ops.Elicit(ctx, &schema.ElicitationRequestParams{
	Message: "Open URL and complete GitHub auth: " + oobURL,
})
if err := waitForToken(ctx, ns, alias, domain); err != nil {
	return authErrorResult("auth timeout/cancel"), nil
}
```
