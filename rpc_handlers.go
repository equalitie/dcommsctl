package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

type InstallInstanceArgs struct {
	AppName    string `json:"name"`
	DomainName string `json:"domain"`
}

type InstallInstanceResult struct {
	JobID      string `json:"job_id"`
	ID         string `json:"id"`
	AdminLogin string `json:"admin_login"`
	AdminPass  string `json:"admin_pass"`
}

func jobLockFile(lockPath string) *os.File {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return f
}

func (app *GlobalState) rpcInstallInstance(ctx context.Context, args InstallInstanceArgs) (InstallInstanceResult, error) {
	applicationID := args.AppName
	_, found := app.systemCfg.Applications[applicationID]

	if !found {
		return InstallInstanceResult{}, fmt.Errorf("We do not support this type of application: %s", applicationID)
	}

	jobType := "install"

	switch applicationID {
	case "peertube":
		{
			instanceDir, err := peertubeCreateInstance(ctx, app.systemCfg.InstancesDir)
			if err != nil {
				return InstallInstanceResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFileName := "." + jobType + ".lock"

			lockFile := jobLockFile(path.Join(instanceDir, lockFileName))

			if lockFile == nil {
				return InstallInstanceResult{}, fmt.Errorf("The job is in progress")
			}
			defer lockFile.Close()

			pass := "test_pass"
			cmd := exec.CommandContext(ctx, path.Join(app.systemCfg.DcommsConfFilesDir, "peertube_install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"LOCK_FILE=" + lockFileName,
				"APP_DIR=" + instanceDir,
				"PEERTUBE_ADMIN_PASS=" + pass,
				"DWEB_DOMAIN=" + args.DomainName,
			}

			if err := app.jobsManager.startExec(ctx, instanceDir, "install", cmd); err != nil {
				return InstallInstanceResult{}, err
			}

			return InstallInstanceResult{
				JobID:      "install",
				ID:         instanceID,
				AdminLogin: "root",
				AdminPass:  pass,
			}, nil
		}
	}
	return InstallInstanceResult{}, nil
}

type RemoveAppArgs struct {
	ID string `json:"id"`
}
type RemoveAppResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcRemoveApp(ctx context.Context, args RemoveAppArgs) (RemoveAppResult, error) {
	return RemoveAppResult{}, nil
}

type StartAppArgs struct {
	InstanceName string `json:"id"`
}

type StartAppResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcStartApp(ctx context.Context, args StartAppArgs) (string, error) {
	return "", nil
}

type StopAppArgs struct {
	InstanceName string `json:"id"`
}

type StopAppResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcStopApp(ctx context.Context, args StopAppArgs) (string, error) {
	return "", nil
}
