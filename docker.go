package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	dockerContainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	dockerClient "github.com/moby/moby/client"

	"github.com/compose-spec/compose-go/v2/cli"
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
	// FIXME: this is the only function that sets up an SSH tunnel
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

func dockerCreateUpdateCompose(ctx context.Context, composeFilePath string) error {
	projectName := "mastodon"

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
	return []string{}, nil
}

func scriptTarStream(
	filename string,
	contents []byte,
) (io.Reader, error) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)

	err := writer.WriteHeader(&tar.Header{
		Name: filename,
		Mode: 0o700,
		Size: int64(len(contents)),
	})
	if err != nil {
		return nil, err
	}

	if _, err := writer.Write(contents); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return bytes.NewReader(buffer.Bytes()), nil
}

func dockerRunBackupRestore(ctx context.Context, app Application, cli *dockerClient.Client, service *ServiceConfig, cmds []string, scriptName string, scriptContents []byte) error {
	helperImage := "postgres:alpine"
	env, err := readEnv(app.systemCfg.Services["peertube"].ConfigFilePath)
	if err != nil {
		return err
	}

	hostBackupDir, err := filepath.Abs("~/.backup")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(hostBackupDir, 0o700); err != nil {
		return fmt.Errorf("create backup dir: %s", err)
	}

	peertubeDbContainer := "postgresql:apline"

	env = append(
		env,
		"BACKUP_DIR=",
	)

	created, err := cli.ContainerCreate(
		ctx,

		dockerClient.ContainerCreateOptions{
			Config: &container.Config{
				Image:        helperImage,
				Cmd:          cmds,
				Env:          env,
				AttachStdout: true,
				AttachStderr: true,
				Tty:          false,
			},
			HostConfig: &container.HostConfig{
				VolumesFrom: []string{peertubeDbContainer + ":ro"},

				Mounts: []mount.Mount{
					{
						Type:   mount.TypeBind,
						Source: hostBackupDir,
						Target: "/backups",
					},
				},
			},
			NetworkingConfig: nil,
			Platform:         nil,
			Name:             "peertube_helper_db",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create helper container: %s", err)
	}

	defer func() {
		cleanupContext, cancel := context.WithTimeout(
			ctx,
			15*time.Second,
		)
		defer cancel()

		_, err = cli.ContainerRemove(
			cleanupContext,
			created.ID,
			dockerClient.ContainerRemoveOptions{Force: true},
		)
	}()

	// This is required by CopyToContainer
	tarStream, err := scriptTarStream(scriptName, scriptContents)
	if err != nil {
		return fmt.Errorf("turning script into tar stream: %s", err)
	}

	if _, err := cli.CopyToContainer(
		ctx,
		created.ID,
		dockerClient.CopyToContainerOptions{
			DestinationPath: "/tools",
			Content:         tarStream,
		},
	); err != nil {
		return fmt.Errorf("upload tools: %s", err)
	}

	attached, err := cli.ContainerAttach(
		ctx,
		created.ID,
		dockerClient.ContainerAttachOptions{
			Stream: true,
			Stdout: true,
			Stderr: true,
		},
	)
	if err != nil {
	}

	var stderr bytes.Buffer
	var stdout bytes.Buffer

	copyErrCh := make(chan error, 1)

	go func() {
		_, copyErr := stdcopy.StdCopy(
			&stdout,
			&stderr,
			attached.Reader,
		)
		copyErrCh <- copyErr
	}()

	containerWaitRes := cli.ContainerWait(
		ctx,
		created.ID,
		dockerClient.ContainerWaitOptions{
			Condition: container.WaitConditionNotRunning,
		},
	)
	var exitCode int64

	select {

	case err := <-containerWaitRes.Error:
		return err
	case result := <-containerWaitRes.Result:
		exitCode = result.StatusCode
	}

	if err := <-copyErrCh; err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf(
			"backup script failed with code %d: %s",
			exitCode,
			strings.TrimSpace(stderr.String()),
		)
	}

	// TODO: figure out how we write this back
	strings.TrimSpace(stdout.String())

	return nil
}
