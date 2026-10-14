package api

import (
	"context"
	"fmt"
	"path"

	"eq/dcommsctl/internal/jobs"
)

type InstanceUpgradeArgs struct {
	InstanceName string `json:"id"`
}

type InstanceUpgradeResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) InstanceUpgrade(ctx context.Context, args InstanceUpgradeArgs) (InstanceUpgradeResult, error) {
	// jobType := "upgrade"

	instanceID := args.InstanceName

	instanceDir := path.Join(app.SystemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(instanceDir, ".job.lock")

	if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
		return InstanceUpgradeResult{}, fmt.Errorf("The job is in progress")
	}

	return InstanceUpgradeResult{
		JobID: "", // jobID,
	}, fmt.Errorf("job unimplemented")
}
