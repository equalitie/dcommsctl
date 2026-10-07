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
	// jobType := "upgrade"

	instanceID := args.InstanceName

	instanceDir := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(instanceDir, ".job.lock")

	if err := jobTryToLockFile(path.Join(instanceDir, lockFilePath)); err != nil {
		return InstanceUpgradeResult{}, fmt.Errorf("The job is in progress")
	}

	return InstanceUpgradeResult{
		JobID: "", // jobID,
	}, fmt.Errorf("job unimplemented")
}
