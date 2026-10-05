package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently-core/protocol/agui"
	"github.com/viant/agently-core/sdk"
	_ "github.com/viant/scy/kms/blowfish"
)

func must(stage string, e error) {
	if e != nil {
		hits := []string{}
		for _, x := range []string{"400", "401", "403", "404", "500", "expired", "signature", "kms", "blowfish", "config", "disabled", "token", "invalid", "scope"} {
			if strings.Contains(strings.ToLower(e.Error()), x) {
				hits = append(hits, x)
			}
		}
		o, _ := json.Marshal(hits)
		fmt.Println(stage + ": FAIL " + string(o))
		os.Exit(1)
	}
	fmt.Println(stage + ": PASS")
}
func main() {
	baseFlag := flag.String("api", "http://127.0.0.1:20431", "Owned isolated backend URL")
	configFlag := flag.String("oauth-config", "", "Scy OAuth client reference")
	secretFlag := flag.String("oob", "", "Scy basic credential reference")
	flag.Parse()
	if *configFlag == "" || *secretFlag == "" {
		fmt.Println("--oauth-config and --oob are required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Timeout: 90 * time.Second}
	base := strings.TrimRight(*baseFlag, "/")
	c, e := sdk.NewHTTP(base, sdk.WithHTTPClient(hc))
	must("SDK client", e)
	must("legacy SDK OOB session", c.AuthLocalOOBSession(ctx, &sdk.LocalOOBSessionOptions{ConfigURL: *configFlag, SecretsURL: *secretFlag, Scopes: []string{"openid", "profile", "email", "plan:create", "plan:edit", "plan:read", "ROLE_STEWARD_WEB"}}))
	_, e = c.AuthMe(ctx)
	must("legacy session identity", e)
	_, e = c.GetPublicAgents(ctx)
	must("legacy public agents", e)
	for _, p := range []string{"web", "ios", "android"} {
		m, e := c.GetWorkspaceMetadataWithTarget(ctx, &sdk.MetadataTargetContext{Platform: p, FormFactor: "phone", Surface: "app"})
		must("legacy "+p+" workspace metadata", e)
		if m.DefaultAgent != "steward" {
			must("Steward default agent", fmt.Errorf("unexpected agent"))
		}
	}
	_, e = c.ListConversations(ctx, &sdk.ListConversationsInput{})
	must("legacy conversations", e)
	for _, op := range []string{"capabilities", "workspace.metadata.get", "workspace.publicagents.list", "workspace.models.list"} {
		input := map[string]any{"threadId": uuid.NewString(), "runId": uuid.NewString(), "messages": []any{}, "tools": []any{}, "context": []any{}, "state": map[string]any{}, "forwardedProps": map[string]any{"agently": map[string]any{"version": "1", "requestId": uuid.NewString(), "operation": op, "payload": map[string]any{}}}}
		b, _ := json.Marshal(input)
		rq, e := http.NewRequestWithContext(ctx, "POST", base+"/v1/ag-ui/run", bytes.NewReader(b))
		must(op+" request", e)
		rq.Header.Set("Content-Type", "application/json")
		resp, e := hc.Do(rq)
		must("AG-UI "+op+" HTTP", e)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			must(op+" HTTP status", fmt.Errorf("status %d", resp.StatusCode))
		}
		s := bufio.NewScanner(resp.Body)
		s.Buffer(make([]byte, 65536), 8<<20)
		finished := false
		n := 0
		for s.Scan() {
			line := s.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			wire := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			e = agui.ValidateEvent(wire)
			if e != nil {
				resp.Body.Close()
				must(op+" schema", e)
			}
			n++
			var event map[string]any
			_ = json.Unmarshal(wire, &event)
			if event["type"] == "RUN_ERROR" {
				resp.Body.Close()
				must(op+" run", fmt.Errorf("run error"))
			}
			if event["type"] == "RUN_FINISHED" {
				finished = true
			}
		}
		resp.Body.Close()
		must(op+" stream", s.Err())
		if !finished {
			must(op+" terminal", fmt.Errorf("missing terminal"))
		}
		fmt.Printf("AG-UI %s: PASS (%d schema-valid events)\n", op, n)
	}
	_, e = c.AuthMe(ctx)
	must("legacy session after AG-UI", e)
}
