package main

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/compose-spec/compose-go/v2/cli"
	dockerClient "github.com/moby/moby/client"
)

func initPeertubeConf(ctx context.Context) (ApplicationConfig, error) {
	app := ctx.Value("app").(*GlobalState)
	composeFilePath := app.systemCfg.DcommsConfFilesDir + "/conf/compose/peertube.docker-compose.yml"

	cfg := ApplicationConfig{
		Name:           "peertube",
		Started:        false,
		ConfigFilePath: app.systemCfg.DcommsConfFilesDir + "/conf/peertube/environment",
		// FIXME: get proper container names from compose files
		BackupSteps: []BackupRestoreStep{
			{
				TmpContainerImage: "",
				ServiceName:       "peertube-postgres",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/peertube_backup_postgres.sh",
			},
			// FIXME: we do not need containers for the last two
			{
				TmpContainerImage: "alpine:3.23",
				ServiceName:       "peertube-back",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/peertube_backup_media.sh",
			},
			{
				TmpContainerImage: "alpine:3.23",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/peertube_backup_final.sh",
			},
		},
		RestoreSteps: []BackupRestoreStep{
			{
				TmpContainerImage: "",
				ServiceName:       "peertube-postgres",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/peertube_restore_postgres.sh",
			},
			{
				TmpContainerImage: "alpine:3.23",
				ServiceName:       "peertube-back",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/peertube_restore_media.sh",
			},
		},
		ComposePath: composeFilePath,
	}

	opt, err := cli.NewProjectOptions(
		[]string{composeFilePath},
		cli.WithOsEnv,
		cli.WithDotEnv,
	)
	if err != nil {
		return ApplicationConfig{}, fmt.Errorf("Composer parser options err: %s", err)
	}

	prj, err := opt.LoadProject(ctx)
	if err != nil {
		return ApplicationConfig{}, err
	}

	srvs, err := prj.GetServices()
	if err != nil {
		return ApplicationConfig{}, nil
	}

	for idx, step := range cfg.BackupSteps {
		if step.ServiceName != "" {
			found := false
			for _, s := range srvs {
				if step.ServiceName == s.Name {
					found = true
					if step.TmpContainerImage == "" {
						cfg.BackupSteps[idx].TmpContainerImage = s.ContainerName
					}
					break
				}
			}

			if !found {
				return ApplicationConfig{}, fmt.Errorf(
					"expected service not found in peertube config: %s. Check config files",
					step.ServiceName,
				)
			}
		}
	}

	for idx, step := range cfg.RestoreSteps {
		if step.ServiceName != "" {
			found := false
			for _, s := range srvs {
				if step.ServiceName == s.Name {
					found = true
					if step.TmpContainerImage == "" {
						cfg.BackupSteps[idx].TmpContainerImage = s.ContainerName
					}
					break
				}
			}

			if !found {
				return ApplicationConfig{}, fmt.Errorf(
					"expected service not found in peertube config: %s. Check config files",
					step.ServiceName,
				)
			}
		}
	}

	return cfg, nil
}

func peertubePostInstall(ctx context.Context, cli *dockerClient.Client) error {
	result, err := cli.ContainerList(ctx, dockerClient.ContainerListOptions{
		All:     true,
		Filters: dockerClient.Filters{},
	})
	if err != nil {
		return fmt.Errorf("search peertube container: %s", err)
	}

	for _, ctr := range result.Items {
		cmd := []string{
			"peertube",
			"npm run reset-password",
			"--",
			"-u",
			"root",
		}

		if err := dockerRun(ctx, cli, ctr.ID, cmd); err != nil {
			fmt.Println(err)
		}
	}

	return nil
}

// This is an test version of calling docker API from the bash script
func peertubeDockerBackup(ctx context.Context, app *GlobalState) error {
	backupRoot := "./.backup"
	// FIXME: we need to store this somewhere and generate it dynamically
	backupID := "tmp1"

	// TODO: we need to parse compose file in advance and get those service names
	cli, err := dockerClient.New(
		dockerClient.FromEnv,
	)

	postgresContainer := ""

	dataContainer := ""

	containers, err := dockerFindContainerByProjectAndService(ctx, cli, "compose_peertube", "peertube-postgres")

	for _, container := range containers.Items {
		postgresContainer = container.ID
	}

	containers, err = dockerFindContainerByProjectAndService(ctx, cli, "compose_peertube", "peertube-back")

	for _, container := range containers.Items {
		dataContainer = container.ID
	}

	cmd := exec.CommandContext(ctx, "bash", "./docker_peertube_backup.sh")

	cmd.Dir = app.systemCfg.DcommsConfFilesDir
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
		"POSTGRES_CONTAINER=" + postgresContainer,
		"DATA_CONTAINER=" + dataContainer,
	}

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))

	if err != nil {
		return err
	}

	return nil
}
