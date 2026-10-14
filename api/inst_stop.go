package api

import (
	"context"
	"fmt"
	"os/exec"
	"path"

	"eq/dcommsctl/internal/jobs"
)

type InstanceStopArgs struct {
	InstanceID string `json:"id"`
}

type InstanceStopResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) InstanceStop(ctx context.Context, args InstanceStopArgs) (InstanceStopResult, error) {
	jobType := "stop"

	instanceID := args.InstanceID

	pathToInst := path.Join(app.SystemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(pathToInst, ".job.lock")

	if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
		return InstanceStopResult{}, fmt.Errorf("The job is in progress")
	}

	cmdArgs := []string{"compose", "down"}
	cmd := exec.CommandContext(app.JobsManager.Context(), "docker", cmdArgs...)
	cmd.Dir = pathToInst

	jobID, err := app.JobsManager.StartExec(ctx, instanceID, lockFilePath, jobType, cmd)
	if err != nil {
		return InstanceStopResult{}, err
	}

	return InstanceStopResult{
		JobID: jobID,
	}, nil
}
