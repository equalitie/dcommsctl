package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

type InstanceBackupArgs struct {
	InstanceName string `json:"id"`
}

type InstanceBackupResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceBackup(ctx context.Context, args InstanceBackupArgs) (InstanceBackupResult, error) {
	jobType := "backup"

	instanceID := args.InstanceName

	pathToInst := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(pathToInst, ".job.lock")

	instanceDirParts := strings.Split(path.Base(pathToInst), "-")

	appName := instanceDirParts[0]
	pathToAppConfig := path.Join(app.systemCfg.AppsDir, appName, "config.json")

	// FIXME: improve the check
	if _, err := os.Stat(pathToAppConfig); err != nil {
		return InstanceBackupResult{}, fmt.Errorf("Application %s is not supported", appName)
	}

	if err := jobTryToLockFile(lockFilePath); err != nil {
		return InstanceBackupResult{}, fmt.Errorf("The job is in progress")
	}

	backupID := fmt.Sprintf("%s-%s", instanceID, time.Now().UTC().Format("01-02.15:04:05"))
	backupRoot := path.Join(app.systemCfg.BackupsDir, appName, instanceID)

	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return InstanceBackupResult{}, err
	}

	cmd := exec.CommandContext(app.jobsManager.ctx, path.Join(app.systemCfg.AppsDir, appName, "backup.sh"))
	cmd.Dir = pathToInst
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
	}

	jobID, err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceBackupResult{}, err
	}

	return InstanceBackupResult{
		JobID: jobID,
	}, nil
}
