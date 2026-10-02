package main

import "context"

type PanicArgs struct{}

type PanicResult struct{}

func (app *GlobalState) rpcPanic(ctx context.Context, args PanicArgs) (PanicResult, error) {
	app.rpcInstanceStop(ctx, InstanceStopArgs{InstanceID: "caddy"})
	return PanicResult{}, nil
}
