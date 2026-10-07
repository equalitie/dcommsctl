package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

type InstanceRestoreArgs struct {
	InstanceName string `json:"instance_id"`
	BackupID     string `json:"backup_id"`
}

type InstanceRestoreResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceRestore(ctx context.Context, args InstanceRestoreArgs) (InstanceRestoreResult, error) {
	jobType := "restore"
	jobID := makeJobID(jobType)

	instanceID := args.InstanceName

	pathToInst := path.Join(app.systemCfg.InstancesDir, instanceID)

	instanceDirParts := strings.Split(path.Base(pathToInst), "-")

	appName := instanceDirParts[0]

	backupID := args.BackupID

	fmt.Println(backupID)

	backupIDSplit := strings.Split(backupID, "-")

	// FIXME: this is unreliable
	instanceFromID := backupIDSplit[0] + "-" + backupIDSplit[1]

	backupPath := path.Join(app.systemCfg.BackupsDir, appName, instanceFromID, backupID)

	if _, err := os.Stat(backupPath); err != nil {
		return InstanceRestoreResult{}, fmt.Errorf("Failed to get access to backup dir: %s", err)
	}

	lockFilePath := path.Join(pathToInst, ".job.lock")
	if err := jobTryToLockFile(lockFilePath); err != nil {
		return InstanceRestoreResult{}, fmt.Errorf("The job is in progress")
	}

	cmd := exec.CommandContext(app.jobsManager.ctx, path.Join(app.systemCfg.AppsDir, appName, "restore.sh"))
	cmd.Dir = pathToInst
	cmd.Env = []string{
		"BACKUP_PATH=" + backupPath,
	}

	fmt.Println(pathToInst)
	jobID, err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceRestoreResult{}, err
	}

	return InstanceRestoreResult{
		JobID: jobID,
	}, nil
}
