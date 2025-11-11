package exec

import (
	"context"
	"fmt"
	"time"

	"github.com/viant/afs/url"
	sys "github.com/viant/agently/genai/tool/service/system"
	"github.com/viant/gosh"
	"github.com/viant/gosh/runner"
	"github.com/viant/gosh/runner/local"
	rssh "github.com/viant/gosh/runner/ssh"
	"golang.org/x/crypto/ssh"

	"strings"

	_ "embed"
	"reflect"

	svc "github.com/viant/agently/genai/tool/service"
	"github.com/viant/scy/cred/secret"
)

//go:embed doc/exec_spec.md
var description string // retained for human docs; not surfaced to LLM

const Name = "system/exec"
const timeoutCode = -101

// Service executes terminal commands
type Service struct{}

type sessionInfo struct {
	id      string
	service *gosh.Service
	close   func()
}

// New creates a new Service instance
func New() *Service { return &Service{} }

// Execute executes terminal commands on the target system
func (s *Service) Execute(ctx context.Context, input *Input, output *Output) error {
	input.Init()
	session, err := s.getSession(ctx, input.Host, input.Env)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	defer session.close()

	if input.Workdir == "" {
		if cmd := input.HasFSCommand(); cmd != "" {
			return fmt.Errorf("workdir is required for %s", cmd)
		}
	}
	if input.Workdir == "." {
		if cmd := input.HasFSCommand(); cmd != "" {
			return fmt.Errorf("absolute path in workdir is required for %s", cmd)
		}
	}

	if input.Workdir != "" {
		_, _, err := session.service.Run(ctx, fmt.Sprintf("cd %s", input.Workdir))
		if err != nil {
			return fmt.Errorf("failed to change directory: %w", err)
		}
	}

	abortOnError := true
	if input.AbortOnError != nil {
		abortOnError = *input.AbortOnError
	}

	commands := make([]*Command, 0, len(input.Commands))
	var combinedStdout, combinedStderr strings.Builder
	var lastExitCode int

	timeoutDuration := time.Duration(input.TimeoutMs) * time.Millisecond
	if timeoutDuration == 0 {
		timeoutDuration = 3 * time.Minute
	}
	var errorCodeCmd string
	var lastErrorCode int
	for _, cmd := range input.Commands {
		command := &Command{Input: cmd}
		stdout, stderr, exitCode := s.executeCommand(ctx, session, cmd, timeoutDuration)
		command.Output = stdout
		command.Stderr = stderr
		command.Status = exitCode
		commands = append(commands, command)

		if exitCode != 0 {
			lastErrorCode = exitCode
			errorCodeCmd = cmd
		}
		if stdout != "" {
			combinedStdout.WriteString(stdout)
			combinedStdout.WriteString("\n")
		}
		if stderr != "" {
			combinedStderr.WriteString(stderr)
			combinedStderr.WriteString("\n")
		}
		lastExitCode = exitCode
		if abortOnError && exitCode != 0 {
			break
		}
	}

	output.Commands = commands
	output.Stdout = strings.TrimSpace(combinedStdout.String())
	output.Stderr = strings.TrimSpace(combinedStderr.String())
	output.Status = lastExitCode
	if lastErrorCode != 0 && output.Stderr == "" {
		output.Stderr = fmt.Sprintf("command %s exited with non-zero exit code", errorCodeCmd)
	}
	return nil
}

func (s *Service) executeCommand(ctx context.Context, session *sessionInfo, command string, duration time.Duration) (string, string, int) {
	started := time.Now()
	stdout, status, err := session.service.Run(ctx, command, runner.WithTimeout(int(duration.Milliseconds())))
	elapsed := time.Since(started)
	if elapsed > duration && err == nil {
		err = fmt.Errorf("command %v timed out after: %s", command, elapsed.String())
		status = timeoutCode
		return stdout, err.Error(), status
	}
	if status == 0 {
		return stdout, "", status
	}
	if stdout == "" && err != nil {
		stdout = err.Error()
	}
	return "", stdout, status
}

func (s *Service) getSession(ctx context.Context, host *sys.Host, env map[string]string) (*sessionInfo, error) {
	sessionID := host.URL
	var service *gosh.Service
	var err error
	envOptions := []runner.Option{}
	if len(env) > 0 {
		envOptions = append(envOptions, runner.WithEnvironment(env))
	}
	if url.Host(host.URL) == "localhost" {
		service, err = gosh.New(ctx, local.New(envOptions...))
	} else {
		config, err := s.getSSHConfig(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("failed to get SSH config: %w", err)
		}
		sshHost := url.Host(host.URL)
		if !strings.Contains(sshHost, ":") {
			sshHost += ":22"
		}
		service, err = gosh.New(ctx, rssh.New(sshHost, config, envOptions...))
	}
	if err != nil {
		return nil, err
	}
	session := &sessionInfo{service: service, id: sessionID, close: func() { _ = service.Close() }}
	return session, nil
}

func (s *Service) getSSHConfig(ctx context.Context, host *sys.Host) (*ssh.ClientConfig, error) {
	credentials := host.Credentials
	if credentials == "" {
		credentials = "localhost"
	}
	secrets := secret.New()
	generic, err := secrets.GetCredentials(ctx, credentials)
	if err != nil {
		return nil, err
	}
	return generic.SSH.Config(ctx)
}

// Name returns the service name.
func (s *Service) Name() string { return Name }

// Methods returns method signatures for this service.
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{{
		Name:        "execute",
		Description: "Run shell commands on local host (no pipes). When using fs operation workdir is required",
		Input:       reflect.TypeOf(&Input{}),
		Output:      reflect.TypeOf(&Output{}),
	}}
}

func (s *Service) execute(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*Input)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*Output)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	return s.Execute(ctx, input, output)
}

// Method resolves an executable by name.
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "execute":
		return s.execute, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}
