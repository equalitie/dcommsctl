package main

import (
	"context"
	"fmt"
	"strings"

	dockerClient "github.com/moby/moby/client"
)

type MatrixConfigData struct {
	AdminName string
	AdminPass string
}

func matrixPostInstall(ctx context.Context, cli *dockerClient.Client) error {
	result, err := cli.ContainerList(ctx, dockerClient.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("Failed to list all docker containers: %s", err)
	}

	for _, ctr := range result.Items {
		// FIXME: just testing execution of commands, no idea for now how to call it directly on container
		if strings.HasPrefix(ctr.Image, "matrixdotorg/synapse:") {
			cmd := []string{
				"sh", "-c",
				// FIXME: gen password and feed it from stdin
				// FIXME: hardcoded passwords
				`register_new_matrix_user -u dcomms-admin -p test --admin -c /data/homeserver.yaml || curl -X POST http://localhost:8008/_matrix/client/v3/login -H "Content-Type: application/json" -d '{ "type": "m.login.password", "identifier": { "type": "m.id.user", "user": "dcomms-admin" }, "password": "test" }'`,
			}

			if err := dockerRun(ctx, cli, ctr.ID, cmd); err != nil {
				fmt.Println(err)
			}
		}
	}

	return nil
}
