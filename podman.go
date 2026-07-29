package main

import (
	"context"
	"log"
	"os"

	"go.podman.io/podman/v6/pkg/bindings"
	// "go.podman.io/podman/v6/pkg/bindings/images"
	// "go.podman.io/podman/v6/pkg/specgen"
	// "github.com/gorilla/websocket"
)

func podmanConnect(rootContext context.Context) context.Context {
	podmanSocket := os.Getenv("PODMAN_SOCKET")

	if podmanSocket == "" {
		log.Fatalln("You need to set PODMAN_SOCKET to a path to socket file for running podman. See run.sh for example")
	}

	ctx, err := bindings.NewConnection(rootContext, "unix://"+podmanSocket)
	if err != nil {
		log.Fatalln(err)
		return nil
	}

	return ctx
}
