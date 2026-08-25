package main

import (
	"context"
	"fmt"
	"os/exec"
	"path"
)

type CreateAppArgs struct {
	AppName    string `json:"name"`
	DomainName string `json:"domain"`
}

type CreateAppResult struct {
	JobID      string `json:"job_id"`
	ID         string `json:"id"`
	AdminLogin string `json:"admin_login"`
	AdminPass  string `json:"admin_pass"`
}

func (app *GlobalState) rpcCreateApp(ctx context.Context, args CreateAppArgs) (CreateAppResult, error) {
	// FIXME:
	// t := DeploymentTargetInfo{}

	applicationID := args.AppName
	_, found := app.systemCfg.Applications[applicationID]

	if !found {
		return CreateAppResult{}, fmt.Errorf("We do not support this type of application: %s", applicationID)
	}

	// app.targets[t.ID] = t

	appID := applicationID + "-111"
	appDir := path.Join("./.apps", appID)

	switch applicationID {
	case "peertube":
		{
			composePrj, err := composeChangePorts(ctx, path.Join(app.systemCfg.DcommsConfFilesDir, "/conf/compose/peertube.docker-compose.yml"), map[string]string{
				"1935": "1936",
				"9010": "9011",
			})
			if err != nil {
				return CreateAppResult{}, nil
			}

			if err := composeStore(ctx, composePrj, path.Join(appDir, "docker-compose.yml")); err != nil {
				return CreateAppResult{}, nil
			}

			pass := "test_pass"
			cmd := exec.CommandContext(ctx, "./peertube_install.sh")
			cmd.Dir = app.systemCfg.DcommsConfFilesDir
			cmd.Env = []string{
				"DCOMMS_DIR=" + app.systemCfg.DcommsConfFilesDir,
				"APP_DIR=" + appDir,
				"PEERTUBE_ADMIN_PASS=" + pass,
				"DWEB_DOMAIN=" + args.DomainName,
			}

			app.jobsManager.startExec(ctx, "dcomms", "create", cmd)
			return CreateAppResult{
				ID:         appID,
				AdminLogin: "root",
				AdminPass:  pass,
			}, nil
		}
	}
	return CreateAppResult{}, nil
}

type RemoveAppArgs struct {
	ID string `json:"id"`
}
type RemoveAppResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcRemoveApp(ctx context.Context, args RemoveAppArgs) (RemoveAppResult, error) {
	return RemoveAppResult{}, nil
}

type StartAppArgs struct {
	InstanceName string `json:"id"`
}

type StartAppResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcStartApp(ctx context.Context, args StartAppArgs) (string, error) {
	cmd := exec.CommandContext(ctx, "./dcomms_compose.sh", "up -d")
	cmd.Dir = app.systemCfg.DcommsConfFilesDir

	if err := app.jobsManager.startExec(ctx, args.InstanceName, "stop", cmd); err != nil {
		return "failed to start app", err
	}

	return "", nil
}

type StopAppArgs struct {
	InstanceName string `json:"id"`
}

type StopAppResult struct {
	JobID string `json:"job_id"`
}

func (app *GlobalState) rpcStopApp(ctx context.Context, args StopAppArgs) (string, error) {
	cmd := exec.CommandContext(ctx, "./dcomms_compose.sh", "up -d")
	cmd.Dir = app.systemCfg.DcommsConfFilesDir

	if err := app.jobsManager.startExec(ctx, args.InstanceName, "stop", cmd); err != nil {
		return "failed to start app", err
	}

	return "", nil
}
