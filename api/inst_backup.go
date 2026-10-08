package api

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"eq/dcommsctl/internal/jobs"
)

type InstanceBackupArgs struct {
	InstanceName string `json:"id"`
}

type InstanceBackupResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) InstanceBackup(ctx context.Context, args InstanceBackupArgs) (InstanceBackupResult, error) {
	jobType := "backup"

	instanceID := args.InstanceName

	pathToInst := path.Join(app.SystemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(pathToInst, ".job.lock")

	instanceDirParts := strings.Split(path.Base(pathToInst), "-")

	appName := instanceDirParts[0]
	pathToAppConfig := path.Join(app.SystemCfg.AppsDir, appName, "config.json")

	// FIXME: improve the check
	if _, err := os.Stat(pathToAppConfig); err != nil {
		return InstanceBackupResult{}, fmt.Errorf("Application %s is not supported", appName)
	}

	if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
		return InstanceBackupResult{}, fmt.Errorf("The job is in progress")
	}

	backupID := fmt.Sprintf("%s-%s", instanceID, time.Now().UTC().Format("01-02.15:04:05"))
	backupRoot := path.Join(app.SystemCfg.BackupsDir, appName, instanceID)

	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return InstanceBackupResult{}, err
	}

	cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, appName, "backup.sh"))
	cmd.Dir = pathToInst
	cmd.Env = []string{
		"BACKUP_ROOT=" + backupRoot,
		"BACKUP_ID=" + backupID,
	}

	jobID, err := app.JobsManager.StartExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceBackupResult{}, err
	}

	return InstanceBackupResult{
		JobID: jobID,
	}, nil
}
