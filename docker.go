package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

func sshDockerList(ctx context.Context, cli *client.Client) error {
	result, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("Failed to list all docker containers: %s", err)
	}

	fmt.Printf("%s  %-22s  %s\n", "ID", "STATUS", "IMAGE")
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
		fmt.Printf("%s  %-22s  %s\n", ctr.ID, ctr.Status, ctr.Image)
	}

	return nil
}

func sshDockerSend(ctx context.Context, t DeploymentTargetInfo, localAddr string) (*client.Client, error) {
	if !t.SSHTunnelEstablished {
		if err := sshTunnel(ctx, t, localAddr); err != nil {
			return nil, err
		}
	}

	containerClient, err := client.New(
		client.FromEnv,
		client.WithHost("tcp://"+localAddr),
	)

	if err != nil {
		return nil, fmt.Errorf("Failed to create docker client: %s", err)
	}
	defer containerClient.Close()

	pingResult, err := containerClient.Ping(ctx, client.PingOptions{})

	if err != nil {
		return nil, fmt.Errorf("Failed to ping docker client: %s", err)
	}

	fmt.Println("Docker API version", pingResult.APIVersion)

	return containerClient, nil
}

func dockerRun(ctx context.Context, cli *client.Client, ctrID string, cmd []string) error {
	createResp, err := cli.ExecCreate(ctx, ctrID, client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
	})

	if err != nil {
		fmt.Println("Failed to create exec command on container:", err)
		return err
	}

	attachResp, err := cli.ExecAttach(ctx, createResp.ID, client.ExecAttachOptions{TTY: false})
	if err != nil {
		fmt.Println("Failed to attach to exec:", err)
		return err
	}
	defer attachResp.Close()
	fmt.Println("--- Running command on container ---")
	// inspect, err := cli.ExecInspect(ctx, createResp.ID, client.ExecInspectOptions{})
	// fmt.Printf("Running: %v, ExitCode: %d\n", inspect.Running, inspect.ExitCode)
	// FIXME: this one hangs
	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, attachResp.Reader)
	if err != nil {
		fmt.Println("Failed to read output:", err)
		return err
	}
	fmt.Println("---     End of command output    ---")

	return nil
}
