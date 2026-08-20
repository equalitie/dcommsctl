package main

import (
	"context"
	"fmt"
	"os/exec"
	// "github.com/compose-spec/compose-go/v2/cli"
)

func initPeertubeConf(ctx context.Context) (ApplicationConfig, error) {
	app := ctx.Value("app").(*GlobalState)
	composeFilePath := app.systemCfg.DcommsConfFilesDir + "/conf/compose/peertube.docker-compose.yml"

	cfg := ApplicationConfig{
		Name:           "peertube",
		Started:        false,
		ConfigFilePath: app.systemCfg.DcommsConfFilesDir + "/conf/peertube/environment",
		ComposePath:    composeFilePath,
	}

	// opt, err := cli.NewProjectOptions(
	// 	[]string{composeFilePath},
	// 	cli.WithOsEnv,
	// 	cli.WithDotEnv,
	// )
	// if err != nil {
	// 	return ApplicationConfig{}, fmt.Errorf("Composer parser options err: %s", err)
	// }

	// prj, err := opt.LoadProject(ctx)
	// if err != nil {
	// 	return ApplicationConfig{}, err
	// }

	// srvs, err := prj.GetServices()
	// if err != nil {
	// 	return ApplicationConfig{}, nil
	// }

	return cfg, nil
}

func peertubePostInstall(ctx context.Context, app *GlobalState) error {
	pass := "test_pass"
	cmd := exec.CommandContext(ctx, "./peertube_postinstall.sh")
	cmd.Dir = app.systemCfg.DcommsConfFilesDir
	cmd.Env = []string{
		"PEERTUBE_ADMIN_PASS=" + pass,
	}

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))

	if err != nil {
		return err
	}

	return nil
}

// This is an test version of calling docker API from the bash script
func peertubeDockerBackup(ctx context.Context, app *GlobalState) error {
	// FIXME: figure out where we store that
	backupRoot := "./.backup"
	// FIXME: we need to store this somewhere and generate it dynamically
	backupID := "tmp1"

	cmd := exec.CommandContext(ctx, "./docker_peertube_backup.sh")
	cmd.Dir = app.systemCfg.DcommsConfFilesDir
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
	}

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))

	if err != nil {
		return err
	}

	return nil
}

// This is an test version of calling docker API from the bash script
func peertubeDockerRestore(ctx context.Context, app *GlobalState, backupID string) error {
	// FIXME: figure out where we store that
	backupRoot := "./.backup"

	cmd := exec.CommandContext(ctx, "./docker_peertube_restore.sh")
	cmd.Dir = app.systemCfg.DcommsConfFilesDir
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
	}

	output, err := cmd.CombinedOutput()

	fmt.Println(string(output))

	if err != nil {
		return err
	}

	return nil
}
