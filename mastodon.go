package main

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/v2/cli"
)

func initMastodonConf(ctx context.Context) (ApplicationConfig, error) {
	app := ctx.Value("app").(*GlobalState)
	composeFilePath := app.systemCfg.DcommsConfFilesDir + "/conf/compose/mastodon.docker-compose.yml"

	cfg := ApplicationConfig{
		Name:           "Mastodon",
		Started:        false,
		ConfigFilePath: app.systemCfg.DcommsConfFilesDir + "/conf/mastodon/env.production",
		BackupSteps: []BackupRestoreStep{
			{
				ServiceName:       "mastodon-db",
				TmpContainerImage: "",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/mastodon_backup_postgres.sh",
			},
			{
				ServiceName:       "mastodon-redis",
				TmpContainerImage: "",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/mastodon_backup_redis.sh",
			},
			{
				ServiceName:       "",
				TmpContainerImage: "alpine:3.23",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/mastodon_backup_media.sh",
			},
			{
				ServiceName:       "",
				TmpContainerImage: "alpine:3.23",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/mastodon_backup_final.sh",
			},
		},
		RestoreSteps: []BackupRestoreStep{
			{
				ServiceName:       "mastodon-db",
				TmpContainerImage: "postgres:14.13-alpine",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/mastodon_restore_postgres.sh",
			},
			{
				ServiceName:       "mastodon-redis",
				TmpContainerImage: "",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/mastodon_restore_redis.sh",
			},
			{
				ServiceName:       "",
				TmpContainerImage: "alpine:3.23",
				ScriptPath:        app.systemCfg.DcommsConfFilesDir + "/mastodon_restore_media.sh",
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
					"expected service not found in mastodon config: %s. Check config files",
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
					"expected service not found in mastodon config: %s. Check config files",
					step.ServiceName,
				)
			}
		}
	}

	return cfg, nil
}
