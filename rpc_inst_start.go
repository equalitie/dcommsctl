package main

import (
	"context"
	"fmt"
	"os/exec"
	"path"
)

type InstanceStartArgs struct {
	InstanceName string `json:"id"`
}

type InstanceStartResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceStart(ctx context.Context, args InstanceStartArgs) (InstanceStartResult, error) {
	jobType := "start"

	jobID := jobID(jobType)

	instanceID := args.InstanceName
	if instanceID == "" {
		return InstanceStartResult{}, fmt.Errorf("No instance specified")
	}

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(instanceDir, "."+jobType+".lock")

	lockFile := jobLockFile(lockFilePath)
	if lockFile == nil {
		return InstanceStartResult{}, fmt.Errorf("The job is in progress")
	}
	lockFile.Close()

	cmdArgs := []string{"compose", "--project-name", instanceID, "up", "-d"}

	cmd := exec.CommandContext(app.jobsManager.ctx, "docker", cmdArgs...)
	cmd.Dir = instanceDir

	if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, jobID, cmd); err != nil {
		return InstanceStartResult{}, err
	}

	return InstanceStartResult{
		JobID: jobID,
	}, nil
}
