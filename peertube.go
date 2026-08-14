package main

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/v2/cli"
)

func initPeertubeConf(ctx context.Context) (ApplicationConfig, error) {
	app := ctx.Value("app").(*GlobalState)
	composeFilePath := app.systemCfg.DcommsConfFilesDir + "/conf/compose/peertube.docker-compose.yml"

	cfg := ApplicationConfig{
		Name:           "Peertube",
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
