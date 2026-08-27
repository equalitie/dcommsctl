package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"text/template"
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
	appDir := ""
	domain := ""
	pass := "test_pass"
	cmd := exec.CommandContext(ctx, "./peertube_install.sh")
	cmd.Dir = app.systemCfg.DcommsConfFilesDir
	cmd.Env = []string{
		"APP_DIR=" + appDir,
		"PEERTUBE_ADMIN_PASS=" + pass,
		"DWEB_DOMAIN=" + domain,
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

type AdminConfig struct {
	Email string
}

type DbConfig struct {
	Password string
}

type PeertubeConfig struct {
	Domain   string
	PortOut  int
	PortOut2 int
	Db       DbConfig
}

func peertubeCreateInstance(ctx context.Context, instancesDir string) (string, error) {
	tmpl, err := template.ParseFiles("./conf_tmpl/peertube/docker-compose.tmpl.yml")
	if err != nil {
		return "", err
	}

	pathToInst, err := instanceCreate(ctx, instancesDir, "peertube")

	composeFile, err := os.OpenFile(path.Join(pathToInst, "docker-compose.yml"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	data := PeertubeConfig{
		Domain:   "peertube.",
		PortOut:  1935,
		PortOut2: 9090,
		Db: DbConfig{
			Password: "",
		},
	}

	if err := tmpl.Execute(composeFile, &data); err != nil {
		return "", err
	}

	envTmpl, err := template.ParseFiles("./conf_tmpl/peertube/env")
	if err != nil {
		return "", err
	}

	envFile, err := os.OpenFile(path.Join(pathToInst, "env"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}

	if err := envTmpl.Execute(envFile, &data); err != nil {
		return "", err
	}

	return pathToInst, nil
}
