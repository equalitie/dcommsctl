package api

import "context"

type InstanceHideArgs struct {
	InstanceID string `json:"id"`
}

type InstanceHideResult struct{}

func InstanceHide(ctx context.Context, args InstanceHideArgs) (InstanceHideResult, error) {
	return InstanceHideResult{}, nil
}
