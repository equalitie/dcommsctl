package main

import (
	"context"
	"fmt"
	"os/exec"
	"path"
)

type InstanceStopArgs struct {
	InstanceID string `json:"id"`
}

type InstanceStopResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceStop(ctx context.Context, args InstanceStopArgs) (InstanceStopResult, error) {
	jobType := "stop"

	instanceID := args.InstanceID

	pathToInst := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(pathToInst, ".job.lock")

	if err := jobTryToLockFile(path.Join(pathToInst, lockFilePath)); err != nil {
		return InstanceStopResult{}, fmt.Errorf("The job is in progress")
	}

	cmdArgs := []string{"compose", "down"}
	cmd := exec.CommandContext(app.jobsManager.ctx, "docker", cmdArgs...)
	cmd.Dir = pathToInst

	jobID, err := app.jobsManager.startExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceStopResult{}, err
	}

	return InstanceStopResult{
		JobID: jobID,
	}, nil
}
