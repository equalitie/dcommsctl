package main

import (
	"context"
	"os"

	"go.podman.io/podman/v6/pkg/bindings"
	// "go.podman.io/podman/v6/pkg/bindings/images"
	// "go.podman.io/podman/v6/pkg/specgen"
	// "github.com/gorilla/websocket"
)

func podmanConnect(ctx context.Context) context.Context {
	podmanSocket := os.Getenv("PODMAN_SOCKET")

	if podmanSocket == "" {
		logFatal("PODMAN_SOCKET must be set to the podman socket path; see run.sh for an example")
		return nil
	}

	c, err := bindings.NewConnection(ctx, "unix://"+podmanSocket)
	if err != nil {
		logFatal("cannot connect to podman", "socket", podmanSocket, "err", err)
		return nil
	}

	return c
}
