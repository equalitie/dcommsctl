package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

type InstanceBackupArgs struct {
	InstanceName string `json:"id"`
}

type InstanceBackupResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceBackup(ctx context.Context, args InstanceBackupArgs) (InstanceBackupResult, error) {
	jobType := "backup"
	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(instanceDir, ".job.lock")

	instanceDirParts := strings.Split(path.Base(instanceDir), "-")

	appName := instanceDirParts[0]
	pathToAppConfig := path.Join(app.systemCfg.AppsDir, appName, "config.json")

	// FIXME: improve the check
	if _, err := os.Stat(pathToAppConfig); err != nil {
		return InstanceBackupResult{}, fmt.Errorf("Application %s is not supported", appName)
	}

	if err := jobTryToLockFile(path.Join(instanceDir, lockFilePath)); err != nil {
		return InstanceBackupResult{}, fmt.Errorf("The job is in progress")
	}

	backupID := instanceID
	backupRoot := path.Join(app.systemCfg.BackupsDir, backupID)

	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return InstanceBackupResult{}, err
	}

	cmd := exec.CommandContext(app.jobsManager.ctx, path.Join(app.systemCfg.AppsDir, appName, "backup.sh"))
	cmd.Dir = instanceDir
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
		"APP_DIR=" + instanceDir,
	}
	if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, jobID, cmd); err != nil {
		return InstanceBackupResult{}, err
	}

	return InstanceBackupResult{
		JobID: jobID,
	}, nil
}
