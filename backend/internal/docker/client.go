// Package docker provides an isolated challenge-environment client.
//
// By default (no build tags) a stub client is returned: all allocation calls
// fail with "docker daemon unavailable" and credits are refunded. To enable the
// real Docker-backed implementation, build with the `docker` build tag and use
// NewDockerClient() instead of NewClient().
//
//go:build !docker

package docker

import (
	"context"
	"fmt"
	"time"
)

// Client is the interface for allocating disposable challenge environments.
type Client interface {
	// Available reports whether a container runtime is reachable.
	Available() bool
	// AllocateContainer starts a disposable container for a challenge.
	// It returns the container ID and a cleanup function.
	AllocateContainer(ctx context.Context, image, name string, env []string, cmd []string) (string, func(), error)
	// Exec runs a command inside a container and returns combined stdout/stderr.
	Exec(ctx context.Context, containerID string, cmd []string) (string, error)
}

// StubClient is a no-op client used when Docker is not available.
type StubClient struct{}

// NewClient returns a stub client when Docker support is not compiled in.
func NewClient() (*StubClient, error) {
	return &StubClient{}, nil
}

func (c *StubClient) Available() bool { return false }

func (c *StubClient) AllocateContainer(ctx context.Context, image, name string, env []string, cmd []string) (string, func(), error) {
	return "", nil, fmt.Errorf("docker daemon unavailable")
}

func (c *StubClient) Exec(ctx context.Context, containerID string, cmd []string) (string, error) {
	return "", fmt.Errorf("docker daemon unavailable")
}

// compile-time check
var _ Client = (*StubClient)(nil)

var _ = time.Now
