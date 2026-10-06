package main

import (
	"context"
	"os/exec"
)

type PanicArgs struct{}

type PanicResult struct{}

func (app *GlobalState) rpcPanic(ctx context.Context, args PanicArgs) (PanicResult, error) {
	cmd := exec.Command("panic.sh")

	if err := cmd.Start(); err != nil {
		return PanicResult{}, nil
	}

	cmd.Wait()

	return PanicResult{}, nil
}
