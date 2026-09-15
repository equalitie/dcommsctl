package main

import "context"

type JobLogArgs struct {
	JobID      string `json:"job_id"`
	InstanceID string `json:"instance_id"`
	Offset     int64  `json:"offset"`
}

type JobLogResult struct {
	Log string `json:"log"`
}

func (app *GlobalState) rpcJobLog(ctx context.Context, args JobLogArgs) (JobLogResult, error) {
	r := JobLogResult{}

	d, err := app.jobsManager.fetchLog(args.InstanceID, args.JobID, args.Offset)
	if err != nil {
		return JobLogResult{}, err
	}

	r.Log = string(d)

	return r, nil
}
