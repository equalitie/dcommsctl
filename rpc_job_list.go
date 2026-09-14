package main

import "context"

type JobsListArgs struct {
	InstanceID string `json:"instance_id"`
	ActiveOnly bool   `json:"active_only"`
}

type JobsListResult struct {
	Jobs []JobStatus `json:"jobs"`
}

func (app *GlobalState) rpcJobsList(ctx context.Context, args JobsListArgs) (JobsListResult, error) {
	instanceID := args.InstanceID
	filterByInstance := instanceID != ""
	r := JobsListResult{}
	for _, j := range app.jobsManager.jobs {
		if filterByInstance {
			if j.status.InstanceID != instanceID {
				continue
			}
		}

		if args.ActiveOnly {
			if !isJobActive(j.status.State) {
				continue
			}
		}

		r.Jobs = append(r.Jobs, j.status)
	}

	return r, nil
}
