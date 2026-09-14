package main

import "context"

type JobStatusArgs struct {
	JobID `json:"id"`
}

func (app *GlobalState) rpcJobStatus(ctx context.Context, args JobStatusArgs) (JobStatus, error) {
	r := JobStatus{}

	return r, nil
}
