package main

import "context"

type PanicArgs struct{}

type PanicResult struct{}

func (app *GlobalState) rpcPanic(ctx context.Context, args PanicArgs) (PanicResult, error) {
	app.rpcInstanceStop(ctx, InstanceStopArgs{InstanceName: "caddy"})
	return PanicResult{}, nil
}
