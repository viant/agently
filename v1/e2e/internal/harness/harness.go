package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	agentlyBinaryOnce sync.Once
	agentlyBinaryPath string
	agentlyBinaryErr  error
)

func RepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func CopyWorkspaceTemplate(t *testing.T, templatePath string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), ".agently")
	if err := copyDir(templatePath, target); err != nil {
		t.Fatalf("copy workspace template: %v", err)
	}
	return target
}

func StartServer(t *testing.T, workspacePath string) string {
	t.Helper()
	addr := freeAddr(t)
	cmd := exec.Command(agentlyBinary(t), "serve", "--addr", addr)
	cmd.Dir = RepoRoot()
	configureProcessGroup(cmd)
	traceFile := DebugLogPath(t, "trace.ndjson")
	_ = os.Remove(traceFile)
	payloadDir := DebugLogPath(t, "llm-payloads")
	_ = os.RemoveAll(payloadDir)
	cmd.Env = append(os.Environ(),
		"AGENTLY_WORKSPACE="+workspacePath,
		"AGENTLY_DEBUG_TRACE_FILE="+traceFile,
		"AGENTLY_DEBUG_PAYLOAD_DIR="+payloadDir,
	)
	logFile := DebugLogPath(t, "server.log")
	logWriter, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create server log: %v", err)
	}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		stopCommand(cmd, 3*time.Second)
		_ = logWriter.Close()
	})
	waitForHealth(t, "http://"+addr+"/healthz", logFile)
	return "http://" + addr
}

func RunChat(t *testing.T, workspacePath, baseURL string, args []string, stdin string) string {
	t.Helper()
	output, err := RunChatResult(t, workspacePath, baseURL, args, stdin)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return output
}

func RunChatResult(t *testing.T, workspacePath, baseURL string, args []string, stdin string) (string, error) {
	t.Helper()
	timeout := 2 * time.Minute
	if requested := requestedChatTimeout(args); requested > 0 {
		timeout = requested + 30*time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmdArgs := append([]string{"chat", "--api", baseURL}, args...)
	cmd := exec.CommandContext(ctx, agentlyBinary(t), cmdArgs...)
	cmd.Dir = RepoRoot()
	configureProcessGroup(cmd)
	cmd.Env = append(os.Environ(), "AGENTLY_WORKSPACE="+workspacePath)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	} else {
		cmd.Stdin = strings.NewReader("")
	}
	logFile := DebugLogPath(t, "chat.log")
	defer stopCommand(cmd, 2*time.Second)
	output, err := cmd.CombinedOutput()
	writeDebugLog(t, logFile, fmt.Sprintf("baseURL=%s\ntimeout=%s\nargs=%q\n\n%s", baseURL, timeout, args, string(output)))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return string(output), fmt.Errorf("run chat timed out after %s (log: %s)\n%s", timeout, logFile, string(output))
		}
		return string(output), fmt.Errorf("run chat failed: %v (log: %s)\n%s", err, logFile, string(output))
	}
	return string(output), nil
}

func requestedChatTimeout(args []string) time.Duration {
	for i := 0; i < len(args); i++ {
		if strings.TrimSpace(args[i]) != "--timeout" {
			continue
		}
		if i+1 >= len(args) {
			return 0
		}
		seconds, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
		if err != nil || seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	return 0
}

func EnsureGitClone(t *testing.T, repoURL, dirName string) string {
	t.Helper()
	cacheRoot := filepath.Join(RepoRoot(), ".e2e-cache", "repos")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatalf("create repo cache dir: %v", err)
	}
	target := filepath.Join(cacheRoot, dirName)
	if isGitRepo(target) {
		return target
	}
	_ = os.RemoveAll(target)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, target)
	cmd.Dir = cacheRoot
	configureProcessGroup(cmd)
	defer stopCommand(cmd, 2*time.Second)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("git clone %s timed out\n%s", repoURL, string(output))
		}
		t.Fatalf("git clone %s failed: %v\n%s", repoURL, err, string(output))
	}
	if !isGitRepo(target) {
		t.Fatalf("git clone did not produce a repository: %s", target)
	}
	return target
}

func agentlyBinary(t *testing.T) string {
	t.Helper()
	cachePath := filepath.Join(RepoRoot(), ".e2e-cache", "agently")
	if !envEnabled("AGENTLY_E2E_REBUILD_BINARY") && isExecutableFile(cachePath) && !binaryStale(cachePath) {
		return cachePath
	}
	agentlyBinaryOnce.Do(func() {
		agentlyBinaryPath = cachePath
		if err := os.MkdirAll(filepath.Dir(agentlyBinaryPath), 0o755); err != nil {
			agentlyBinaryErr = fmt.Errorf("create e2e cache dir: %w", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", agentlyBinaryPath, "./agently")
		cmd.Dir = RepoRoot()
		configureProcessGroup(cmd)
		defer stopCommand(cmd, 2*time.Second)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				agentlyBinaryErr = fmt.Errorf("build agently e2e binary timed out\n%s", string(output))
				return
			}
			agentlyBinaryErr = fmt.Errorf("build agently e2e binary: %w\n%s", err, string(output))
			return
		}
	})
	if agentlyBinaryErr != nil {
		t.Fatalf("%v", agentlyBinaryErr)
	}
	return agentlyBinaryPath
}

func binaryStale(binaryPath string) bool {
	info, err := os.Stat(binaryPath)
	if err != nil {
		return true
	}
	binaryTime := info.ModTime()
	for _, root := range rebuildWatchRoots() {
		newer, walkErr := treeHasNewerFile(root, binaryTime)
		if walkErr != nil || newer {
			return true
		}
	}
	return false
}

func rebuildWatchRoots() []string {
	repoRoot := RepoRoot()
	parent := filepath.Dir(repoRoot)
	return []string{
		filepath.Join(repoRoot, "agently"),
		filepath.Join(repoRoot, "cmd"),
		filepath.Join(repoRoot, "v1"),
		filepath.Join(repoRoot, "go.mod"),
		filepath.Join(repoRoot, "go.sum"),
		filepath.Join(parent, "agently-core"),
	}
}

func treeHasNewerFile(root string, after time.Time) (bool, error) {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return info.ModTime().After(after), nil
	}
	var newer bool
	err = filepath.Walk(root, func(path string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() {
			switch name {
			case ".git", ".e2e-cache", "node_modules", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if entry.ModTime().After(after) {
			newer = true
			return io.EOF
		}
		return nil
	})
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return newer, err
}

func envEnabled(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return enabled
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func waitForHealth(t *testing.T, url, logPath string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && strings.Contains(string(body), `"status":"ok"`) {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	data, _ := os.ReadFile(logPath)
	t.Fatalf("server did not become healthy: %s", string(data))
}

func DebugLogPath(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(RepoRoot(), ".e2e-cache", "logs", sanitizeTestName(t.Name()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create debug log dir: %v", err)
	}
	return filepath.Join(dir, name)
}

func writeDebugLog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write debug log %s: %v", path, err)
	}
}

func sanitizeTestName(name string) string {
	name = strings.TrimSpace(name)
	replacer := strings.NewReplacer("/", "_", " ", "_", ":", "_", "\\", "_")
	return replacer.Replace(name)
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info != nil
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func Context(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func configureProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopCommand(cmd *exec.Cmd, waitTimeout time.Duration) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	default:
	}
	_ = terminateProcessGroup(cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-done:
		return
	case <-time.After(waitTimeout):
		_ = terminateProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(waitTimeout):
		}
	}
}

func terminateProcessGroup(pid int, signal syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, signal)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if perr := syscall.Kill(pid, signal); perr == nil || errors.Is(perr, syscall.ESRCH) {
		return nil
	}
	return err
}
