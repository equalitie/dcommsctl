package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"

	"eq/dcommsctl/internal/jobs"
	"eq/dcommsctl/internal/proxy"
)

type InstanceExposeArgs struct {
	InstanceID string `json:"id"`
}

type InstanceExposeResult struct{}

func (app *GlobalState) InstanceExpose(ctx context.Context, args InstanceExposeArgs) (InstanceExposeResult, error) {
	instanceID := args.InstanceID
	if instanceID == "" {
		return InstanceExposeResult{}, fmt.Errorf("No instance specified")
	}

	pathToInst := path.Join(app.SystemCfg.InstancesDir, instanceID)
	lockFilePath := path.Join(pathToInst, ".job.lock")

	if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
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
	if _, _, err := proxy.CaddySendRequest(ctx, endpoint, http.MethodPost, "", caddyRouteJSON); err != nil {
		return InstanceExposeResult{}, err
	}

	if err := proxy.ExportConfig(ctx, app.SystemCfg.InstancesDir); err != nil {
		return InstanceExposeResult{}, err
	}

	return InstanceExposeResult{}, nil
}
