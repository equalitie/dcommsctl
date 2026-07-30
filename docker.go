package main

import (
	"context"
	"fmt"
	"os"
	"io"
	"encoding/json"

	"github.com/moby/moby/api/pkg/stdcopy"
	dockerClient "github.com/moby/moby/client"
	dockerContainer "github.com/moby/moby/api/types/container"
)

func sshDockerInspectContainers(ctx context.Context, cli *dockerClient.Client) error {
	result, err := cli.ContainerList(ctx, dockerClient.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("Failed to list all docker containers: %s", err)
	}

	for _, ctr := range result.Items {
		info, err := cli.ContainerInspect(ctx, ctr.ID, dockerClient.ContainerInspectOptions{})
		if err != nil {
			fmt.Println(err)
			continue
		}

		r := info.Container.HostConfig.Resources
		// NOTE: printing random stuff for now just to figure out what we might need
		resp, err := cli.ContainerStats(ctx, ctr.ID, dockerClient.ContainerStatsOptions{})

		if err != nil {
			fmt.Println(err)
			continue
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var stats dockerContainer.StatsResponse
		if err := json.Unmarshal(data, &stats); err != nil {
			fmt.Println(err)
			continue
		}

		ramUsed := float32(stats.MemoryStats.Usage) / float32(stats.MemoryStats.Limit)

		fmt.Printf("%-22s %d %d %d %d %d %f\n", ctr.Image,info.Container.SizeRw, r.Memory, r.NanoCPUs, r.CPUQuota, r.CPUShares, ramUsed)
	}

	return nil
}

func sshDockerListContainers(ctx context.Context, cli *dockerClient.Client) error {
	result, err := cli.ContainerList(ctx, dockerClient.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("Failed to list all docker containers: %s", err)
	}

	fmt.Printf("%s  %-22s  %s\n", "ID", "STATUS", "IMAGE")
	for _, ctr := range result.Items {
		fmt.Printf("%s  %-22s  %s\n", ctr.ID, ctr.Status, ctr.Image)
	}

	return nil
}

func sshDockerSend(ctx context.Context, t DeploymentTargetInfo, localAddr string) (*dockerClient.Client, error) {
	if !t.SSHTunnelEstablished {
		if err := sshTunnel(ctx, t, localAddr); err != nil {
			return nil, err
		}
	}

	containerClient, err := dockerClient.New(
		dockerClient.FromEnv,
		dockerClient.WithHost("tcp://"+localAddr),
	)

	if err != nil {
		return nil, fmt.Errorf("Failed to create docker client: %s", err)
	}
	defer containerClient.Close()

	pingResult, err := containerClient.Ping(ctx, dockerClient.PingOptions{})

	if err != nil {
		return nil, fmt.Errorf("Failed to ping docker client: %s", err)
	}

	fmt.Println("Docker API version", pingResult.APIVersion)

	return containerClient, nil
}

func dockerRun(ctx context.Context, cli *dockerClient.Client, ctrID string, cmd []string) error {
	createResp, err := cli.ExecCreate(ctx, ctrID, dockerClient.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
	})

	if err != nil {
		fmt.Println("Failed to create exec command on container:", err)
		return err
	}

	attachResp, err := cli.ExecAttach(ctx, createResp.ID, dockerClient.ExecAttachOptions{TTY: false})
	if err != nil {
		fmt.Println("Failed to attach to exec:", err)
		return err
	}
	defer attachResp.Close()
	fmt.Println("--- Running command on container ---")
	// inspect, err := cli.ExecInspect(ctx, createResp.ID, dockerClient.ExecInspectOptions{})
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
