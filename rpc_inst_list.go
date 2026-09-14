package main

import (
	"context"
	"os"
)

type InstancesListArgs struct{}

type InstancesListResult struct {
	InstancesNames []string `json:""`
}

func (app *GlobalState) rpcInstancesList(ctx context.Context, args InstancesListArgs) (InstancesListResult, error) {
	r := InstancesListResult{}
	entries, err := os.ReadDir(app.systemCfg.InstancesDir)
	if err != nil {
		return InstancesListResult{}, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			r.InstancesNames = append(r.InstancesNames, entry.Name())
		}
	}

	return r, nil
}
