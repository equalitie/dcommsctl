package main

import (
	"context"
	"fmt"
	"path"
)

type InstanceUpgradeArgs struct {
	InstanceName string `json:"id"`
}

type InstanceUpgradeResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcInstanceUpgrade(ctx context.Context, args InstanceUpgradeArgs) (InstanceUpgradeResult, error) {
	jobType := "upgrade"
	jobID := jobID(jobType)

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := "." + jobType + ".lock"

	lockFile := jobLockFile(path.Join(instanceDir, lockFilePath))

	if lockFile == nil {
		return InstanceUpgradeResult{}, fmt.Errorf("The job is in progress")
	}

	return InstanceUpgradeResult{
		JobID: jobID,
	}, nil
}
