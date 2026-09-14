package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/moby/moby/api/pkg/stdcopy"
	dockerContainer "github.com/moby/moby/api/types/container"
	dockerClient "github.com/moby/moby/client"

	"github.com/compose-spec/compose-go/v2/cli"

	"github.com/joho/godotenv"
)

func dockerInspectContainers(ctx context.Context, cli *dockerClient.Client) error {
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
		// NOTE: printing random stuff for now just to fiorgure out what we might need
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

		fmt.Printf("%-22s %d %d %d %d %d %f\n", ctr.Image, info.Container.SizeRw, r.Memory, r.NanoCPUs, r.CPUQuota, r.CPUShares, ramUsed)
	}

	return nil
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

func dockerCreateUpdateCompose(ctx context.Context, composeFilePath string) error {
	projectName := "mastodon-update"

	options, err := cli.NewProjectOptions(
		[]string{composeFilePath},
		cli.WithOsEnv,
		cli.WithDotEnv,
		cli.WithName(projectName),
	)
	if err != nil {
		return err
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return err
	}

	projectYAML, err := project.MarshalYAML()
	if err != nil {
		return err
	}

	fmt.Println(projectYAML)

	return nil
}

func readEnv(envFile string) ([]string, error) {
	fileReader, err := os.Open(envFile)
	if err != nil {
		return nil, err
	}

	env, err := godotenv.Parse(fileReader)
	if err != nil {
		return nil, err
	}

	res := []string{}

	for key, val := range env {
		res = append(res, fmt.Sprintf("%s=%s", key, val))
	}

	return res, nil
}

func scriptTarStream(filenames []string) (io.Reader, error) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)

	defer writer.Close()

	for _, filename := range filenames {
		contents, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		if err := writer.WriteHeader(&tar.Header{
			Name: filename,
			Mode: 0o700,
			Size: int64(len(contents)),
		}); err != nil {
			return nil, err
		}

		if _, err := writer.Write(contents); err != nil {
			return nil, err
		}

		if err := writer.Close(); err != nil {
			return nil, err
		}
	}
	return bytes.NewReader(buffer.Bytes()), nil
}

func dockerFindContainerByProjectAndService(ctx context.Context, cli *dockerClient.Client, projectName, serviceName string) (dockerClient.ContainerListResult, error) {
	result, err := cli.ContainerList(ctx, dockerClient.ContainerListOptions{
		Filters: dockerClient.Filters{
			"label": {
				"com.docker.compose.project=" + projectName: true,
				"com.docker.compose.service=" + serviceName: true,
			},
		},
		All: true,
	})
	if err != nil {
		return dockerClient.ContainerListResult{}, fmt.Errorf("find docker container: %s", err)
	}

	return result, nil
}
