package main

import (
	"context"
	"fmt"
	"os/exec"
	"path"
)

type InstanceStartArgs struct {
	InstanceID string `json:"id"`
}

type InstanceStartResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceStart(ctx context.Context, args InstanceStartArgs) (InstanceStartResult, error) {
	jobType := "start"

	instanceID := args.InstanceID
	if instanceID == "" {
		return InstanceStartResult{}, fmt.Errorf("No instance specified")
	}

	pathToInst := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(pathToInst, ".job.lock")

	if err := jobTryToLockFile(lockFilePath); err != nil {
		return InstanceStartResult{}, fmt.Errorf("The job is in progress")
	}

	cmdArgs := []string{"compose", "--project-name", instanceID, "up", "--wait", "--wait-timeout", "120"}

	cmd := exec.CommandContext(app.jobsManager.ctx, "docker", cmdArgs...)
	cmd.Dir = pathToInst

	jobID, err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceStartResult{}, err
	}

	return InstanceStartResult{
		JobID: jobID,
	}, nil
}
