package api

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"eq/dcommsctl/internal/jobs"
)

type InstanceRestoreArgs struct {
	InstanceName string `json:"instance_id"`
	BackupID     string `json:"backup_id"`
}

type InstanceRestoreResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) InstanceRestore(ctx context.Context, args InstanceRestoreArgs) (InstanceRestoreResult, error) {
	jobType := "restore"

	instanceID := args.InstanceName

	pathToInst := path.Join(app.SystemCfg.InstancesDir, instanceID)

	instanceDirParts := strings.Split(path.Base(pathToInst), "-")

	appName := instanceDirParts[0]

	backupID := args.BackupID

	fmt.Println(backupID)

	backupIDSplit := strings.Split(backupID, "-")

	// FIXME: this is unreliable
	instanceFromID := backupIDSplit[0] + "-" + backupIDSplit[1]

	backupPath := path.Join(app.SystemCfg.BackupsDir, appName, instanceFromID, backupID)

	if _, err := os.Stat(backupPath); err != nil {
		return InstanceRestoreResult{}, fmt.Errorf("Failed to get access to backup dir: %s", err)
	}

	lockFilePath := path.Join(pathToInst, ".job.lock")
	if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
		return InstanceRestoreResult{}, fmt.Errorf("The job is in progress")
	}

	cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, appName, "restore.sh"))
	cmd.Dir = pathToInst
	cmd.Env = []string{
		"BACKUP_PATH=" + backupPath,
	}

	fmt.Println(pathToInst)
	jobID, err := app.JobsManager.StartExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceRestoreResult{}, err
	}

	return InstanceRestoreResult{
		JobID: jobID,
	}, nil
}
