package main

import "context"

type JobLogArgs struct {
	JobID `json:"id"`
}

type JobLogResult struct {
	Log string `json:"log"`
}

func rpcJobLog(ctx context.Context, args JobLogArgs) (JobLogResult, error) {
	r := JobLogResult{}

	return r, nil
}
