package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
)

type InstanceRestoreArgs struct {
	InstanceName string `json:"id"`
}

type InstanceRestoreResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceRestore(ctx context.Context, args InstanceRestoreArgs) (InstanceRestoreResult, error) {
	jobType := "restore"
	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := "." + jobType + ".lock"

	lockFile := jobLockFile(path.Join(instanceDir, lockFilePath))

	if lockFile == nil {
		return InstanceRestoreResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()
	backupID := instanceID
	backupRoot := path.Join(app.systemCfg.BackupsDir, backupID)

	if _, err := os.Stat(backupRoot); err != nil {
		return InstanceRestoreResult{}, fmt.Errorf("Failed to get access to backup dir: %s", err)
	}

	cmd := exec.CommandContext(ctx, path.Join("conf_tmpl", "peertube", "restore.sh"))
	cmd.Dir = instanceDir
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
		"APP_DIR=" + instanceDir,
	}
	if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobID, cmd); err != nil {
		return InstanceRestoreResult{}, err
	}

	return InstanceRestoreResult{
		JobID: jobID,
	}, nil
}
