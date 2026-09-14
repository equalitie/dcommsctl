package main

import "context"

type InstanceExposeArgs struct {
	InstanceID string `json:"id"`
}

type InstanceExposeResult struct{}

func rpcInstanceExpose(ctx context.Context, args InstanceExposeArgs) (InstanceExposeResult, error) {
	return InstanceExposeResult{}, nil
}
