package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
)

type InstanceExposeArgs struct {
	InstanceID string `json:"id"`
}

type InstanceExposeResult struct{}

func (app *GlobalState) rpcInstanceExpose(ctx context.Context, args InstanceExposeArgs) (InstanceExposeResult, error) {
	instanceID := args.InstanceID
	if instanceID == "" {
		return InstanceExposeResult{}, fmt.Errorf("No instance specified")
	}

	pathToInst := path.Join(app.systemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(pathToInst, ".job.lock")

	if err := jobTryToLockFile(lockFilePath); err != nil {
		return InstanceExposeResult{}, fmt.Errorf("The job is in progress")
	}

	defer os.Remove(lockFilePath)

	caddyRouteFilePath := path.Join(pathToInst, "caddy-route.json")

	caddyFile, err := os.OpenFile(caddyRouteFilePath, os.O_RDONLY, 0o644)
	if err != nil {
		return InstanceExposeResult{}, err
	}
	defer caddyFile.Close()

	caddyRouteJSON, err := os.ReadFile(caddyRouteFilePath)

	endpoint := "http://localhost:2019/config/apps/http/servers/srv0/routes"
	if _, err := caddySendRequest(ctx, endpoint, http.MethodPost, caddyRouteJSON); err != nil {
		return InstanceExposeResult{}, err
	}

	if err := caddyExportConfig(ctx, app.systemCfg.InstancesDir); err != nil {
		return InstanceExposeResult{}, err
	}

	return InstanceExposeResult{}, nil
}
