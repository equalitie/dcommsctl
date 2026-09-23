package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
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
	lockFilePath := "." + jobType + ".lock"

	lockFile := jobLockFile(path.Join(instanceDir, lockFilePath))

	if lockFile == nil {
		return InstanceBackupResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()

	backupID := instanceID
	backupRoot := path.Join(app.systemCfg.BackupsDir, backupID)

	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return InstanceBackupResult{}, err
	}

	cmd := exec.CommandContext(app.jobsManager.ctx, path.Join("conf_tmpl", "peertube", "backup.sh"))
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
