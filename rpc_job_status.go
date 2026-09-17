package main

import (
	"context"
	"fmt"
)

type JobStatusArgs struct {
	JobID `json:"id"`
}

func (app *GlobalState) rpcJobStatus(ctx context.Context, args JobStatusArgs) (JobStatus, error) {
	status, found := app.jobsManager.statusByID(string(args.JobID))
	if !found {
		return JobStatus{}, fmt.Errorf("unknown job: %s", args.JobID)
	}

	return status, nil
}
