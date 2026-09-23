package main

import (
	"context"
	"fmt"
	"os/exec"
	"path"
)

type InstanceUninstallArgs struct {
	ID string `json:"id"`
}
type InstanceUninstallResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceUninstall(ctx context.Context, args InstanceUninstallArgs) (InstanceUninstallResult, error) {
	jobType := "uninstall"

	jobID := jobID(jobType)

	instanceID := args.ID

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(instanceDir, "."+jobType+".lock")

	lockFile := jobLockFile(lockFilePath)

	if lockFile == nil {
		return InstanceUninstallResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()

	cmdArgs := []string{"compose", "down"}

	cmd := exec.CommandContext(app.jobsManager.ctx, "docker", cmdArgs...)
	cmd.Dir = instanceDir

	if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, jobID, cmd); err != nil {
		return InstanceUninstallResult{}, err
	}

	return InstanceUninstallResult{
		JobID: jobID,
	}, nil
}
