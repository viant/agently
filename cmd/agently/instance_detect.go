package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/viant/agently-core/sdk"
)

type authProviderInfo struct {
	Name            string `json:"name"`
	Label           string `json:"label"`
	Type            string `json:"type"`
	DefaultUsername string `json:"defaultUsername,omitempty"`
}

type instanceInfo struct {
	BaseURL       string
	Port          int
	WorkspaceRoot string
	DefaultAgent  string
	DefaultModel  string
	Models        []string
	Providers     []authProviderInfo
	// ElicitationTimeout is the per-prompt response timeout advertised by the
	// server. Zero means the client should fall back to its built-in default.
	ElicitationTimeout time.Duration
}

func detectLocalInstances(ctx context.Context) ([]*instanceInfo, error) {
	ports := map[int]struct{}{}
	for _, res := range processesFromPS() {
		if res.Port > 0 {
			ports[res.Port] = struct{}{}
			continue
		}
		for _, port := range portsFromPID(res.PID) {
			ports[port] = struct{}{}
		}
	}
	if len(ports) == 0 {
		return nil, nil
	}
	out := make([]*instanceInfo, 0, len(ports))
	for port := range ports {
		if port <= 0 || port > 65535 {
			continue
		}
		baseURL := fmt.Sprintf("http://localhost:%d", port)
		meta, ok := fetchWorkspaceMetadata(ctx, baseURL)
		providers, providersErr := fetchAuthProviders(ctx, baseURL)
		if !ok && providersErr != nil {
			continue
		}
		if meta == nil {
			meta = &workspaceMetadata{}
		}
		out = append(out, &instanceInfo{
			BaseURL:            baseURL,
			Port:               port,
			WorkspaceRoot:      meta.WorkspaceRoot,
			DefaultAgent:       meta.DefaultAgent,
			DefaultModel:       meta.DefaultModel,
			Models:             meta.Models,
			Providers:          providers,
			ElicitationTimeout: meta.ElicitationTimeout,
		})
	}
	return out, nil
}

type processInfo struct {
	PID  int
	Port int
}

func processesFromPS() []processInfo {
	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var procs []processInfo
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	// Command lines can be long; bump the scanner buffer beyond the 64 KiB default.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		pid, rest, ok := splitPIDAndArgs(scanner.Text())
		if !ok {
			continue
		}
		args := tokenizeCommand(rest)
		if !isAgentlyServeProcess(args) {
			continue
		}
		procs = append(procs, processInfo{PID: pid, Port: parsePortFromArgs(args)})
	}
	return procs
}

// splitPIDAndArgs splits a `ps -o pid=,args=` line into its numeric PID and the
// trailing raw command line. Returns ok=false when the line is empty or the
// leading token is not a positive integer.
func splitPIDAndArgs(line string) (int, string, bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	start := i
	for i < len(line) && line[i] != ' ' && line[i] != '\t' {
		i++
	}
	if start == i {
		return 0, "", false
	}
	pid, err := strconv.Atoi(line[start:i])
	if err != nil || pid <= 0 {
		return 0, "", false
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return pid, line[i:], true
}

// tokenizeCommand is a minimal quote-aware tokenizer for `ps` argv output. It
// respects single and double quotes so that paths containing spaces remain a
// single token. Backslash escapes are not interpreted — `ps` does not emit
// them for its own quoting.
func tokenizeCommand(s string) []string {
	var tokens []string
	var cur strings.Builder
	quote := byte(0)
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case c == '"' || c == '\'':
			quote = c
		case c == ' ' || c == '\t':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return tokens
}

func isAgentlyServeProcess(args []string) bool {
	if len(args) == 0 {
		return false
	}
	hasServe := false
	hasAgently := false
	for _, arg := range args {
		token := strings.TrimSpace(arg)
		if token == "" {
			continue
		}
		if token == "serve" {
			hasServe = true
			continue
		}
		base := filepath.Base(trimQuotes(token))
		if base == "agently" {
			hasAgently = true
		}
	}
	return hasServe && hasAgently
}

func parsePortFromArgs(args []string) int {
	addr := extractAddr(args)
	return parsePort(addr)
}

func portsFromPID(pid int) []int {
	if pid <= 0 {
		return nil
	}
	cmd := exec.Command("lsof", "-nP", "-p", strconv.Itoa(pid), "-iTCP", "-sTCP:LISTEN")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var ports []int
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if strings.HasPrefix(line, "COMMAND") {
				continue
			}
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		name := lsofNameField(fields)
		port := parsePortFromLsofName(name)
		if port == 0 {
			continue
		}
		ports = append(ports, port)
	}
	return ports
}

func extractAddr(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--addr=") {
			return trimQuotes(strings.TrimSpace(strings.TrimPrefix(arg, "--addr=")))
		}
		if strings.HasPrefix(arg, "-a=") {
			return trimQuotes(strings.TrimSpace(strings.TrimPrefix(arg, "-a=")))
		}
		if strings.HasPrefix(arg, "-a:") {
			return trimQuotes(strings.TrimSpace(strings.TrimPrefix(arg, "-a:")))
		}
		if arg == "--addr" || arg == "-a" {
			if i+1 < len(args) {
				return trimQuotes(strings.TrimSpace(args[i+1]))
			}
		}
	}
	return ""
}

func parsePort(addr string) int {
	v := trimQuotes(strings.TrimSpace(addr))
	if v == "" {
		return 0
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		v = strings.TrimPrefix(strings.TrimPrefix(v, "http://"), "https://")
	}
	if i := strings.LastIndex(v, ":"); i != -1 {
		v = v[i+1:]
	}
	v = strings.TrimSpace(trimQuotes(v))
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func parsePortFromLsofName(name string) int {
	if name == "" {
		return 0
	}
	name = trimQuotes(strings.TrimSpace(name))
	if i := strings.LastIndex(name, ":"); i != -1 {
		name = name[i+1:]
	}
	name = strings.TrimLeft(name, "*")
	var digits strings.Builder
	for _, r := range name {
		if r < '0' || r > '9' {
			break
		}
		digits.WriteRune(r)
	}
	if digits.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(digits.String())
	return n
}

func trimQuotes(v string) string {
	return strings.Trim(v, "\"'`")
}

func lsofNameField(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	for i := len(fields) - 1; i >= 0; i-- {
		f := strings.TrimSpace(fields[i])
		if f == "" {
			continue
		}
		if strings.Contains(f, ":") && strings.IndexFunc(f, func(r rune) bool { return r >= '0' && r <= '9' }) != -1 {
			return f
		}
	}
	return fields[len(fields)-1]
}

type workspaceMetadata struct {
	WorkspaceRoot      string
	DefaultAgent       string
	DefaultModel       string
	Models             []string
	ElicitationTimeout time.Duration
}

func fetchWorkspaceMetadata(ctx context.Context, baseURL string) (*workspaceMetadata, bool) {
	client, err := sdk.NewHTTP(baseURL)
	if err != nil {
		return nil, false
	}
	meta, err := client.GetWorkspaceMetadata(ctx)
	if err != nil {
		return nil, false
	}
	if meta == nil {
		return nil, false
	}
	if strings.TrimSpace(meta.WorkspaceRoot) == "" && strings.TrimSpace(meta.DefaultAgent) == "" && strings.TrimSpace(meta.DefaultModel) == "" && len(meta.Models) == 0 {
		return nil, false
	}
	var elicitationTimeout time.Duration
	if meta.Defaults != nil && meta.Defaults.ElicitationTimeoutSec > 0 {
		elicitationTimeout = time.Duration(meta.Defaults.ElicitationTimeoutSec) * time.Second
	}
	return &workspaceMetadata{
		WorkspaceRoot:      strings.TrimSpace(meta.WorkspaceRoot),
		DefaultAgent:       strings.TrimSpace(meta.DefaultAgent),
		DefaultModel:       strings.TrimSpace(meta.DefaultModel),
		Models:             meta.Models,
		ElicitationTimeout: elicitationTimeout,
	}, true
}

func fetchAuthProviders(ctx context.Context, baseURL string) ([]authProviderInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/api/auth/providers", nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("auth/providers: %s", resp.Status)
	}
	var payload []authProviderInfo
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}
