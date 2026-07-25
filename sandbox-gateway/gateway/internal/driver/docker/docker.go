package docker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"sandbox-gateway/internal/driver"
	"sandbox-gateway/internal/logging"
)

// cmdRunner executes docker CLI commands (overridable in unit tests).
type cmdRunner func(ctx context.Context, timeout time.Duration, args ...string) (string, error)

// Driver runs sandboxes as containers via the host docker CLI. It publishes the
// image's ports on the host (bindIP:ephemeral) so clients connect directly.
type Driver struct {
	bindIP     string
	network    string
	namePrefix string
	shmSize    string
	run        cmdRunner
}

// Options configures the docker driver.
type Options struct {
	BindIP     string
	Network    string
	NamePrefix string
	ShmSize    string
}

// New builds a docker driver.
func New(o Options) *Driver {
	if o.BindIP == "" {
		o.BindIP = "127.0.0.1"
	}
	if o.NamePrefix == "" {
		o.NamePrefix = "sbx-"
	}
	if o.ShmSize == "" {
		o.ShmSize = "1g"
	}
	return &Driver{
		bindIP:     o.BindIP,
		network:    o.Network,
		namePrefix: o.NamePrefix,
		shmSize:    o.ShmSize,
		run:        run,
	}
}

func (d *Driver) Name() string { return "docker" }

// containerName derives the docker container name from a gateway id.
func (d *Driver) containerName(id string) string { return d.namePrefix + id }

func (d *Driver) Create(ctx context.Context, spec driver.Spec) (*driver.Handle, error) {
	name := d.containerName(spec.ID)

	// --privileged is required for the sandbox image's inner dockerd (DinD).
	// Without it startup.sh fails unless SKIP_INNER_DOCKER=1.
	args := []string{
		"run", "-d", "--name", name, "--privileged",
		"--add-host", "host.docker.internal:host-gateway",
		"--shm-size=" + d.shmSize,
	}
	if spec.Resources.CPUCores > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", spec.Resources.CPUCores))
	}
	if spec.Resources.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", spec.Resources.MemoryMB))
	}
	if spec.WorkspaceDir != "" {
		args = append(args, "-e", "WORKSPACE_DIR="+spec.WorkspaceDir)
	}
	// Publish each requested port on bindIP with an ephemeral host port.
	for _, p := range spec.Ports {
		args = append(args, "-p", fmt.Sprintf("%s::%d", d.bindIP, p))
	}
	if d.network != "" {
		args = append(args, "--network", d.network)
	}
	// Config injection: same-host bind-mount, or SANDBOX_INJECT for URL bundles.
	if spec.Config != nil {
		root := spec.Config.ConfigRoot
		switch {
		case spec.Config.HostPath != "":
			dst := root
			if dst == "" {
				dst = "/root/.cursor"
			}
			args = append(args, "-v", spec.Config.HostPath+":"+dst+":rw")
		case spec.Config.BundleURL != "":
			inject := spec.Config.BundleURL
			if root != "" {
				inject += "|" + root
			}
			args = append(args, "-e", "SANDBOX_INJECT="+inject)
			if spec.Config.Headers != "" {
				args = append(args, "-e", "SANDBOX_INJECT_HEADERS="+spec.Config.Headers)
			}
		}
	}
	for k, v := range spec.Env {
		args = append(args, "-e", k+"="+v)
	}
	for _, mt := range spec.Mounts {
		args = append(args, "-v", mt)
	}
	args = append(args, spec.Image)

	if _, err := d.run(ctx, 90*time.Second, args...); err != nil {
		return nil, fmt.Errorf("docker run: %w", err)
	}

	eps, err := d.endpoints(ctx, name, spec.Ports)
	if err != nil {
		logging.WarnErr(d.destroyByName(context.Background(), name, true), "docker create rollback destroy", map[string]any{
			"container": name,
		})
		return nil, err
	}
	return &driver.Handle{
		ID:        spec.ID,
		Name:      name,
		Status:    driver.StatusRunning,
		Endpoints: eps,
	}, nil
}

func (d *Driver) Start(ctx context.Context, id string) error {
	_, err := d.run(ctx, 30*time.Second, "start", d.containerName(id))
	return err
}

func (d *Driver) Stop(ctx context.Context, id string) error {
	_, err := d.run(ctx, 30*time.Second, "stop", d.containerName(id))
	return err
}

func (d *Driver) Destroy(ctx context.Context, id string) error {
	return d.destroyByName(ctx, d.containerName(id), true)
}

// Reinstall removes the container and creates it again. preserveData keeps
// anonymous volumes (docker rm without -v); false passes -v to wipe them.
// Named host bind-mounts from Spec.Mounts / Config.HostPath are never deleted.
func (d *Driver) Reinstall(ctx context.Context, spec driver.Spec, preserveData bool) error {
	name := d.containerName(spec.ID)
	logging.WarnErr(d.destroyByName(ctx, name, !preserveData), "docker reinstall pre-destroy", map[string]any{
		"container":     name,
		"preserve_data": preserveData,
	})
	_, err := d.Create(ctx, spec)
	return err
}

func (d *Driver) destroyByName(ctx context.Context, name string, removeVolumes bool) error {
	args := []string{"rm", "-f"}
	if removeVolumes {
		args = append(args, "-v")
	}
	args = append(args, name)
	_, err := d.run(ctx, 30*time.Second, args...)
	if isNoSuchContainer(err) {
		return nil
	}
	return err
}

func isNoSuchContainer(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such container")
}

func (d *Driver) Get(ctx context.Context, id string) (*driver.Handle, error) {
	name := d.containerName(id)
	st, err := d.status(ctx, name)
	if err != nil {
		return nil, err
	}
	h := &driver.Handle{ID: id, Name: name, Status: st}
	if st == driver.StatusRunning {
		// Ports are discovered from docker inspect (all published mappings).
		eps, err := d.endpoints(ctx, name, nil)
		if err == nil {
			h.Endpoints = eps
		}
	}
	return h, nil
}

func (d *Driver) List(ctx context.Context) ([]*driver.Handle, error) {
	out, err := d.run(ctx, 15*time.Second, "ps", "-a",
		"--filter", "name="+d.namePrefix,
		"--format", "{{.Names}}\t{{.State}}\t{{.CreatedAt}}")
	if err != nil {
		return nil, err
	}
	var handles []*driver.Handle
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		id := strings.TrimPrefix(name, d.namePrefix)
		h := &driver.Handle{
			ID:     id,
			Name:   name,
			Status: mapState(strings.TrimSpace(parts[1])),
		}
		if len(parts) >= 3 {
			h.CreatedAt = parseDockerCreatedAt(parts[2])
		}
		handles = append(handles, h)
	}
	return handles, nil
}

func parseDockerCreatedAt(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05 -0700 MST",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (d *Driver) Status(ctx context.Context, id string) (driver.Status, error) {
	return d.status(ctx, d.containerName(id))
}

func (d *Driver) status(ctx context.Context, name string) (driver.Status, error) {
	out, err := d.run(ctx, 10*time.Second, "inspect", "--format", "{{.State.Status}}", name)
	if err != nil {
		return driver.StatusNotFound, nil
	}
	return mapState(strings.TrimSpace(out)), nil
}

func (d *Driver) Endpoints(ctx context.Context, id string) (map[int]string, error) {
	return d.endpoints(ctx, d.containerName(id), nil)
}

// Logs returns combined PID1 stdout/stderr via `docker logs --tail` (non-follow).
func (d *Driver) Logs(ctx context.Context, id string, tail int) (string, error) {
	if tail <= 0 {
		tail = 5000
	}
	name := d.containerName(id)
	out, err := d.run(ctx, 30*time.Second, "logs", "--tail", fmt.Sprintf("%d", tail), name)
	if err != nil {
		if isNoSuchContainer(err) {
			return "", fmt.Errorf("sandbox %s not found", id)
		}
		return "", fmt.Errorf("docker logs: %w", err)
	}
	return out, nil
}

// endpoints returns container-port -> "bindIP:hostPort". When ports is nil it
// discovers all published tcp ports from docker inspect.
func (d *Driver) endpoints(ctx context.Context, name string, ports []int) (map[int]string, error) {
	if len(ports) > 0 {
		out := map[int]string{}
		for _, p := range ports {
			hp, err := d.hostPort(ctx, name, p)
			if err != nil {
				return nil, err
			}
			out[p] = fmt.Sprintf("%s:%d", d.bindIP, hp)
		}
		return out, nil
	}
	// Discover all published ports: "<cport>/tcp -> <hostIP>:<hostPort>" lines.
	out, err := d.run(ctx, 10*time.Second, "inspect", "--format",
		`{{range $p, $conf := .NetworkSettings.Ports}}{{if $conf}}{{$p}}={{(index $conf 0).HostPort}}{{"\n"}}{{end}}{{end}}`, name)
	if err != nil {
		return nil, err
	}
	res := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		portProto, hostPort, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		cportStr, _, _ := strings.Cut(portProto, "/")
		var cport, hp int
		if _, err := fmt.Sscanf(strings.TrimSpace(cportStr), "%d", &cport); err != nil {
			continue
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(hostPort), "%d", &hp); err != nil || hp == 0 {
			continue
		}
		res[cport] = fmt.Sprintf("%s:%d", d.bindIP, hp)
	}
	return res, nil
}

func (d *Driver) hostPort(ctx context.Context, name string, containerPort int) (int, error) {
	format := fmt.Sprintf(`{{(index (index .NetworkSettings.Ports "%d/tcp") 0).HostPort}}`, containerPort)
	out, err := d.run(ctx, 10*time.Second, "inspect", "--format", format, name)
	if err != nil {
		return 0, fmt.Errorf("docker inspect port %d: %w", containerPort, err)
	}
	var port int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &port); err != nil || port == 0 {
		return 0, fmt.Errorf("parse host port from %q", out)
	}
	return port, nil
}

func mapState(s string) driver.Status {
	switch s {
	case "running":
		return driver.StatusRunning
	case "created", "restarting", "paused":
		return driver.StatusPending
	case "exited", "dead", "stopped":
		return driver.StatusStopped
	case "", "not_found":
		return driver.StatusNotFound
	default:
		return driver.StatusStopped
	}
}

// run executes a docker CLI command with a timeout, returning trimmed stdout.
func run(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
