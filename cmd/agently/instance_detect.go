package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
}

type workspaceMetadataResponse struct {
	Status string `json:"status"`
	Data   struct {
		WorkspaceRoot string `json:"workspaceRoot"`
		Defaults      struct {
			Agent string `json:"agent"`
			Model string `json:"model"`
		} `json:"defaults"`
		Models []string `json:"models"`
	} `json:"data"`
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
		if !ok {
			continue
		}
		providers, _ := fetchAuthProviders(ctx, baseURL)
		out = append(out, &instanceInfo{
			BaseURL:       baseURL,
			Port:          port,
			WorkspaceRoot: meta.WorkspaceRoot,
			DefaultAgent:  meta.DefaultAgent,
			DefaultModel:  meta.DefaultModel,
			Models:        meta.Models,
			Providers:     providers,
		})
	}
	return out, nil
}

func pidsFromPS() []int {
	// deprecated: kept for compatibility with older callers
	var out []int
	for _, res := range processesFromPS() {
		out = append(out, res.PID)
	}
	return out
}

type processInfo struct {
	PID  int
	Port int
}

func processesFromPS() []processInfo {
	cmd := exec.Command("ps", "-ax", "-o", "pid=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var procs []processInfo
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		cmdline := strings.Join(fields[1:], " ")
		if !strings.Contains(cmdline, "agently") || !strings.Contains(cmdline, "serve") {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || pid <= 0 {
			continue
		}
		port := parsePortFromArgs(fields[1:])
		procs = append(procs, processInfo{PID: pid, Port: port})
	}
	return procs
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
	WorkspaceRoot string
	DefaultAgent  string
	DefaultModel  string
	Models        []string
}

func fetchWorkspaceMetadata(ctx context.Context, baseURL string) (*workspaceMetadata, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/workspace/metadata", nil)
	if err != nil {
		return nil, false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, false
	}
	var payload workspaceMetadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false
	}
	if strings.ToLower(strings.TrimSpace(payload.Status)) != "ok" {
		return nil, false
	}
	return &workspaceMetadata{
		WorkspaceRoot: strings.TrimSpace(payload.Data.WorkspaceRoot),
		DefaultAgent:  strings.TrimSpace(payload.Data.Defaults.Agent),
		DefaultModel:  strings.TrimSpace(payload.Data.Defaults.Model),
		Models:        payload.Data.Models,
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
