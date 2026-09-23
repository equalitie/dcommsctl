package main

import (
	"context"
	"fmt"
	"os/exec"
	"path"
)

type InstanceStopArgs struct {
	InstanceName string `json:"id"`
}

type InstanceStopResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceStop(ctx context.Context, args InstanceStopArgs) (InstanceStopResult, error) {
	jobType := "stop"
	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := "." + jobType + ".lock"

	lockFile := jobLockFile(path.Join(instanceDir, lockFilePath))

	if lockFile == nil {
		return InstanceStopResult{}, fmt.Errorf("The job is in progress")
	}
	defer lockFile.Close()

	cmdArgs := []string{"compose", "down"}

	cmd := exec.CommandContext(app.jobsManager.ctx, "docker", cmdArgs...)
	cmd.Dir = instanceDir
	if err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, jobID, cmd); err != nil {
		return InstanceStopResult{}, err
	}

	return InstanceStopResult{
		JobID: jobID,
	}, nil
}
