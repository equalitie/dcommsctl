package api

import (
	"context"
	"fmt"

	"eq/dcommsctl/internal/jobs"
)

type JobStatusArgs struct {
	jobs.JobID `json:"id"`
}

func (app *GlobalState) JobStatus(ctx context.Context, args JobStatusArgs) (jobs.JobStatus, error) {
	status, found := app.JobsManager.StatusByID(string(args.JobID))
	if !found {
		return jobs.JobStatus{}, fmt.Errorf("unknown job: %s", args.JobID)
	}

	return status, nil
}
