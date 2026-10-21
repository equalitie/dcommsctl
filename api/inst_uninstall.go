package api

import (
	"context"
	"fmt"
	"os/exec"
	"path"

	"eq/dcommsctl/internal/jobs"
)

type InstanceUninstallArgs struct {
	ID string `json:"id"`
	// KeepLogs bool `json:"keep_logs"`
}
type InstanceUninstallResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) InstanceUninstall(ctx context.Context, args InstanceUninstallArgs) (InstanceUninstallResult, error) {
	jobType := "uninstall"

	instanceID := args.ID

	instanceDir := path.Join(app.SystemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(instanceDir, ".job.lock")

	if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
		return InstanceUninstallResult{}, fmt.Errorf("The job is in progress")
	}

	cmdArgs := []string{"compose", "down", "-v", "--remove-orphans"}

	cmd := exec.CommandContext(app.JobsManager.Context(), "docker", cmdArgs...)
	cmd.Dir = instanceDir

	jobID, err := app.JobsManager.StartExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceUninstallResult{}, err
	}

	return InstanceUninstallResult{
		JobID: jobID,
	}, nil
}
