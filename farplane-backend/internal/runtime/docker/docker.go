// Package docker implements runtime.Runtime with the Docker CLI.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/lanetemplate"
	"github.com/farplane/farplane/farplane-backend/internal/runtime"
)

const bridgePort = 7420

// Adapter talks to Docker via the docker CLI (no SDK dependency).
type Adapter struct {
	DockerBin string
	HTTP      *http.Client

	mu          sync.Mutex
	bridgeHosts map[string]string // containerID -> host:port
	envCache    map[string]map[string]string
}

// New builds a Docker runtime adapter.
func New() *Adapter {
	return &Adapter{
		DockerBin:   "docker",
		HTTP:        &http.Client{Timeout: 30 * time.Second},
		bridgeHosts: make(map[string]string),
		envCache:    make(map[string]map[string]string),
	}
}

func (a *Adapter) docker(ctx context.Context, args ...string) *exec.Cmd {
	bin := a.DockerBin
	if bin == "" {
		bin = "docker"
	}
	return exec.CommandContext(ctx, bin, args...)
}

// Create starts a detached container from imageRef with bridge port published.
func (a *Adapter) Create(ctx context.Context, req runtime.CreateRequest) (runtime.Instance, error) {
	name := req.Name
	if name == "" {
		name = "farplane-lane-" + req.LaneID
	}
	args := []string{
		"run", "-d",
		"--name", name,
		"-p", fmt.Sprintf("127.0.0.1::%d", bridgePort),
		"-e", fmt.Sprintf("BRIDGE_PORT=%d", bridgePort),
	}
	for k, v := range req.Env {
		args = append(args, "-e", k+"="+v)
	}
	for k, v := range req.Labels {
		args = append(args, "--label", k+"="+v)
	}
	args = append(args, req.ImageReference)

	out, err := a.docker(ctx, args...).CombinedOutput()
	if err != nil {
		return runtime.Instance{}, fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(string(out)))
	}
	id := strings.TrimSpace(string(out))
	hostPort, err := a.mappedBridgePort(ctx, id)
	if err != nil {
		return runtime.Instance{}, err
	}
	a.mu.Lock()
	a.bridgeHosts[id] = hostPort
	a.envCache[id] = cloneMap(req.Env)
	a.mu.Unlock()

	return runtime.Instance{
		ID:        id,
		Status:    "running",
		BridgeURL: "http://" + hostPort,
	}, nil
}

func (a *Adapter) Destroy(ctx context.Context, id string) error {
	out, err := a.docker(ctx, "rm", "-f", id).CombinedOutput()
	a.mu.Lock()
	delete(a.bridgeHosts, id)
	delete(a.envCache, id)
	a.mu.Unlock()
	if err != nil {
		return fmt.Errorf("docker rm: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) Start(ctx context.Context, id string) error {
	out, err := a.docker(ctx, "start", id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker start: %w: %s", err, strings.TrimSpace(string(out)))
	}
	hostPort, err := a.mappedBridgePort(ctx, id)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.bridgeHosts[id] = hostPort
	a.mu.Unlock()
	return nil
}

func (a *Adapter) Stop(ctx context.Context, id string) error {
	out, err := a.docker(ctx, "stop", id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

type cliExecSession struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (s *cliExecSession) Wait() (int, error) {
	err := s.cmd.Wait()
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func (s *cliExecSession) Stdout() io.Reader { return s.stdout }
func (s *cliExecSession) Stderr() io.Reader { return s.stderr }
func (s *cliExecSession) Close() error {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return nil
}

func (a *Adapter) Exec(ctx context.Context, id string, cmd runtime.ExecRequest) (runtime.ExecSession, error) {
	args := []string{"exec"}
	for k, v := range cmd.Env {
		args = append(args, "-e", k+"="+v)
	}
	if cmd.WorkDir != "" {
		args = append(args, "-w", cmd.WorkDir)
	}
	args = append(args, id)
	args = append(args, cmd.Command...)
	c := a.docker(ctx, args...)
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("docker exec start: %w", err)
	}
	return &cliExecSession{cmd: c, stdout: stdout, stderr: stderr}, nil
}

// InjectSecrets updates secrets for a running Lane.
// Create already passes secrets as docker -e environment variables. Docker cannot
// mutate env on a live container, so this path also writes /run/farplane/secrets.env
// which the agent bridge loads before each turn (and merges into process.env).
func (a *Adapter) InjectSecrets(ctx context.Context, id string, secrets map[string]string) error {
	a.mu.Lock()
	prev := a.envCache[id]
	if prev == nil {
		prev = map[string]string{}
	}
	merged := cloneMap(prev)
	for k, v := range secrets {
		merged[k] = v
	}
	a.envCache[id] = merged
	a.mu.Unlock()

	var b strings.Builder
	for k, v := range secrets {
		fmt.Fprintf(&b, "export %s=%q\n", k, v)
	}
	script := b.String()
	write := a.docker(ctx, "exec", "-i", id, "sh", "-c",
		"mkdir -p /run/farplane && cat > /run/farplane/secrets.env && chmod 600 /run/farplane/secrets.env")
	write.Stdin = strings.NewReader(script)
	out, err := write.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inject secrets: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *Adapter) PreviewURL(ctx context.Context, id string, port int) (string, error) {
	out, err := a.docker(ctx, "port", id, fmt.Sprintf("%d/tcp", port)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker port: %w: %s", err, strings.TrimSpace(string(out)))
	}
	line := strings.TrimSpace(string(out))
	// format: 0.0.0.0:32768
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", fmt.Errorf("no published port for %d", port)
	}
	hostport := parts[0]
	if strings.HasPrefix(hostport, "0.0.0.0:") {
		hostport = "127.0.0.1:" + strings.TrimPrefix(hostport, "0.0.0.0:")
	}
	return "http://" + hostport, nil
}

func (a *Adapter) EnsureAgentBridge(ctx context.Context, id string) error {
	host, err := a.bridgeBase(ctx, id)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("bridge health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge health status %d", resp.StatusCode)
	}
	return nil
}

type sseStream struct {
	ch     chan runtime.AgentEvent
	cancel context.CancelFunc
	resp   *http.Response
}

func (s *sseStream) Events() <-chan runtime.AgentEvent { return s.ch }
func (s *sseStream) Close() error {
	s.cancel()
	if s.resp != nil {
		_ = s.resp.Body.Close()
	}
	return nil
}

func (a *Adapter) OpenAgentStream(ctx context.Context, id string) (runtime.AgentStream, error) {
	host, err := a.bridgeBase(ctx, id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/events", nil)
	if err != nil {
		cancel()
		return nil, err
	}
	// Long-lived stream; use a client without short timeout.
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open agent stream: %w", err)
	}
	s := &sseStream{ch: make(chan runtime.AgentEvent, 32), cancel: cancel, resp: resp}
	go func() {
		defer close(s.ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			var raw map[string]any
			if err := json.Unmarshal([]byte(payload), &raw); err != nil {
				continue
			}
			ev := runtime.AgentEvent{Payload: raw}
			if t, ok := raw["type"].(string); ok {
				ev.Type = t
			}
			if r, ok := raw["role"].(string); ok {
				ev.Role = r
			}
			if b, ok := raw["body"].(string); ok {
				ev.Body = b
			}
			if st, ok := raw["status"].(string); ok {
				ev.Status = st
			}
			if sid, ok := raw["provider_session_id"].(string); ok {
				ev.ProviderSessionID = sid
			}
			if d, ok := raw["done"].(bool); ok {
				ev.Done = d
			}
			select {
			case s.ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return s, nil
}

func (a *Adapter) SendUserTurn(ctx context.Context, id string, turn runtime.UserTurn) error {
	host, err := a.bridgeBase(ctx, id)
	if err != nil {
		return err
	}
	if turn.Type == "" {
		turn.Type = "user_turn"
	}
	body, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/turn", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("send user turn: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send user turn status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// InterruptTurn asks the in-Lane bridge to SIGTERM the active agent process.
func (a *Adapter) InterruptTurn(ctx context.Context, id string) error {
	host, err := a.bridgeBase(ctx, id)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/interrupt", nil)
	if err != nil {
		return err
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("interrupt turn: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("interrupt turn status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// BuildImage writes a temp build context (Dockerfile + embedded bridge) and runs docker build.
func (a *Adapter) BuildImage(ctx context.Context, dockerfileText string, tag string) (string, string, error) {
	if tag == "" {
		tag = fmt.Sprintf("farplane-lane:%d", time.Now().Unix())
	}
	dir, err := os.MkdirTemp("", "farplane-lane-build-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfileText), 0o644); err != nil {
		return "", "", err
	}
	if err := copyBuildContext(dir); err != nil {
		return "", "", err
	}

	cmd := a.docker(ctx, "build", "-t", tag, dir)
	out, err := cmd.CombinedOutput()
	logText := string(out)
	if err != nil {
		return "", logText, fmt.Errorf("docker build: %w", err)
	}
	return tag, logText, nil
}

func copyBuildContext(dest string) error {
	return fs.WalkDir(lanetemplate.BuildContextFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." || path == "Dockerfile" {
			return nil
		}
		target := filepath.Join(dest, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(lanetemplate.BuildContextFS(), path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func (a *Adapter) mappedBridgePort(ctx context.Context, id string) (string, error) {
	out, err := a.docker(ctx, "port", id, fmt.Sprintf("%d/tcp", bridgePort)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker port bridge: %w: %s", err, strings.TrimSpace(string(out)))
	}
	line := strings.TrimSpace(string(out))
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", fmt.Errorf("bridge port not published")
	}
	hostport := parts[0]
	hostport = strings.Replace(hostport, "0.0.0.0:", "127.0.0.1:", 1)
	hostport = strings.Replace(hostport, "[::]:", "127.0.0.1:", 1)
	return hostport, nil
}

func (a *Adapter) bridgeBase(ctx context.Context, id string) (string, error) {
	a.mu.Lock()
	host := a.bridgeHosts[id]
	a.mu.Unlock()
	if host == "" {
		var err error
		host, err = a.mappedBridgePort(ctx, id)
		if err != nil {
			return "", err
		}
		a.mu.Lock()
		a.bridgeHosts[id] = host
		a.mu.Unlock()
	}
	return "http://" + host, nil
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// SetCachedEnv records env for a container id (used after InjectSecrets recreate).
func (a *Adapter) SetCachedEnv(id string, env map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.envCache[id] = cloneMap(env)
}
