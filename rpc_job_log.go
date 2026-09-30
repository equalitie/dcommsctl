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
	Log string `json:"log"`
	// NextOffset is the cursor to pass on the next read.
	NextOffset int64 `json:"next_offset"`

	// Done:
	//
	// True means the job has ended and this reply reaches the end of its output,
	// so nothing more will ever be appended.
	//
	// False means "there may be more later", which is the answer for a job that
	// is still running even when the chunk is empty.
	Done bool `json:"done"`
}

func (app *GlobalState) rpcJobLog(ctx context.Context, args JobLogArgs) (JobLogResult, error) {
	r := JobLogResult{}

	_, found := app.jobsManager.instanceIDForJob(args.JobID)
	if !found {
		return JobLogResult{}, fmt.Errorf("unknown instance for job: %s", args.JobID)
	}

	l, err := app.jobsManager.fetchLog(args.JobID, args.Offset)
	if err != nil {
		return JobLogResult{}, err
	}

	r.Log = string(l.LogChunk)
	r.Done = l.Finished

	return r, nil
}
