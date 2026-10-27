//go:build docker

package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/client"
)

// realClient wraps the Docker API client used to allocate challenge environments.
type realClient struct {
	*client.Client
}

// NewDockerClient creates a Docker client backed by the local daemon.
func NewDockerClient() (*realClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	if _, err := cli.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("docker daemon unreachable: %w", err)
	}
	return &realClient{Client: cli}, nil
}

// Client interface compliance is satisfied by *realClient via embedded *client.Client.
// Available always returns true for the real implementation.
func (c *realClient) Available() bool { return true }

func (c *realClient) AllocateContainer(ctx context.Context, image, name string, env []string, cmd []string) (string, func(), error) {
	containerName := fmt.Sprintf("kubefix-%s-%d", name, time.Now().UnixNano())

	resp, err := c.ContainerCreate(ctx, &client.ContainerConfig{
		Image: image,
		Env:   env,
		Cmd:   cmd,
		Tty:   true,
	}, &client.HostConfig{
		AutoRemove:  true,
		NetworkMode: "bridge",
	}, nil, nil, containerName)
	if err != nil {
		return "", nil, fmt.Errorf("container create failed: %w", err)
	}

	if err := c.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		_ = c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
		return "", nil, fmt.Errorf("container start failed: %w", err)
	}

	cleanup := func() {
		_ = c.ContainerKill(ctx, resp.ID, "SIGTERM")
		_ = c.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
	}

	return resp.ID, cleanup, nil
}

func (c *realClient) Exec(ctx context.Context, containerID string, cmd []string) (string, error) {
	resp, err := c.ContainerExecCreate(ctx, containerID, client.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	})
	if err != nil {
		return "", fmt.Errorf("exec create failed: %w", err)
	}

	hijack, err := c.ContainerExecAttach(ctx, resp.ID, client.ExecAttachOptions{
		Tty: true,
	})
	if err != nil {
		return "", fmt.Errorf("exec attach failed: %w", err)
	}
	defer hijack.Close()

	var out strings.Builder
	_, err = io.Copy(&out, hijack.Reader)
	if err != nil {
		return out.String(), fmt.Errorf("exec output read failed: %w", err)
	}
	return out.String(), nil
}
