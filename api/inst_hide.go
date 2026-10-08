package api

import (
	"context"

	"eq/dcommsctl/internal/proxy"
)

type InstanceHideArgs struct {
	InstanceID string `json:"id"`
}

type InstanceHideResult struct{}

func InstanceHide(ctx context.Context, args InstanceHideArgs) (InstanceHideResult, error) {
	if err := proxy.DropRoute(ctx, args.InstanceID); err != nil {
		return InstanceHideResult{}, err
	}

	return InstanceHideResult{}, nil
}
