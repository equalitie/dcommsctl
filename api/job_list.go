package api

import (
	"context"

	"eq/dcommsctl/internal/jobs"
)

type JobsListArgs struct {
	InstanceID string `json:"instance_id"`
	ActiveOnly bool   `json:"active_only"`
}

type JobsListResult struct {
	Jobs []jobs.JobStatus `json:"jobs"`
}

func (app *GlobalState) JobsList(ctx context.Context, args JobsListArgs) (JobsListResult, error) {
	instanceID := args.InstanceID
	filterByInstance := instanceID != ""
	r := JobsListResult{Jobs: []jobs.JobStatus{}}
	for _, j := range app.JobsManager.Jobs() {
		if filterByInstance {
			if j.Status.InstanceID != instanceID {
				continue
			}
		}

		if args.ActiveOnly {
			if !jobs.IsJobActive(j.Status.State) {
				continue
			}
		}

		r.Jobs = append(r.Jobs, j.Status)
	}

	return r, nil
}
