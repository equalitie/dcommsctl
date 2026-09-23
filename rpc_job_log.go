package main

import (
	"context"
	"fmt"
)

type JobLogArgs struct {
	JobID  string `json:"id"`
	Offset int64  `json:"offset"`
}

type JobLogResult struct {
	Log  string `json:"log"`
	Done bool   `json:"done"`
}

func (app *GlobalState) rpcJobLog(ctx context.Context, args JobLogArgs) (JobLogResult, error) {
	r := JobLogResult{}

	instanceID, found := app.jobsManager.instanceIDForJob(args.JobID)
	if !found {
		return JobLogResult{}, fmt.Errorf("unknown job: %s", args.JobID)
	}

	l, err := app.jobsManager.fetchLog(instanceID, args.JobID, args.Offset)
	if err != nil {
		return JobLogResult{}, err
	}

	r.Log = string(l.LogChunk)
	r.Done = l.LastChunk

	return r, nil
}
