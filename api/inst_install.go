package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"

	"eq/dcommsctl/internal/application"
	"eq/dcommsctl/internal/inst"
	"eq/dcommsctl/internal/jobs"
)

type InstanceInstallArgs struct {
	AppName    string                     `json:"name"`
	DomainName string                     `json:"domain"`
	Variables  map[string]json.RawMessage `json:"variables"`
}

type InstanceInstallResult struct {
	JobID        string `json:"job_id"`
	ID           string `json:"id"`
	AdminLogin   string `json:"admin_login"`
	AdminPass    string `json:"admin_pass"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func randomBytes(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (app *GlobalState) InstanceInstall(ctx context.Context, args InstanceInstallArgs) (InstanceInstallResult, error) {
	jobType := "install"
	clientID, _ := randomBytes(24)
	clientSecret, _ := randomBytes(24)

	if !slices.Contains(application.AppIDs, args.AppName) && !slices.Contains(application.PrivateAppIDS, args.AppName) {
		return InstanceInstallResult{}, fmt.Errorf("This application is not supported, %s", args.AppName)
	}

	appDir := path.Join(app.SystemCfg.AppsDir, args.AppName)

	appConfig, err := application.LoadConfig(path.Join(appDir, "config.json"))
	if err != nil {
		return InstanceInstallResult{}, fmt.Errorf("Failed to read config.json for the app %s: %s", args.AppName, err)
	}

	// Check if the single instance app is already installed
	if appConfig.SingleInstance {
		if _, err := os.Stat(path.Join(app.SystemCfg.InstancesDir, args.AppName)); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return InstanceInstallResult{
					ID: args.AppName,
				}, nil
			}
		} else {
			return InstanceInstallResult{
				ID: args.AppName,
			}, nil
		}
	}

	envVars, installOnlyVars, err := application.ValidateParamsAgainstConfig(args.Variables, appConfig)
	if err != nil {
		return InstanceInstallResult{}, fmt.Errorf("Failed to validate variables for app %s", err)
	}

	instConfig := inst.InstanceGenericConfig{
		SSOIssuerURI: app.SystemCfg.SSODomain,
		Domain:       args.DomainName,
		Env:          envVars,
		Version:      appConfig.Version,
		PlainHTTP:    app.ProxyCfg.PlainHTTP,
	}
	instanceDir, err := inst.InstanceSetup(args.AppName, app.SystemCfg.InstancesDir, instConfig)
	if err != nil {
		return InstanceInstallResult{}, err
	}

	instanceID := filepath.Base(instanceDir)

	installScriptPath := path.Join(appDir, "install.sh")

	if _, err := os.Stat(installScriptPath); err != nil {
		// we do not have an install script, so the installation is done
		if os.IsNotExist(err) {
			return InstanceInstallResult{
				JobID:        "",
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				AdminLogin:   "root",
				AdminPass:    "",
			}, nil
		} else {
			return InstanceInstallResult{
				ID: instanceID,
			}, err
		}
	} else {
		lockFilePath := path.Join(instanceDir, ".job.lock")

		if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
			return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
		}

		envs := []string{
			"VERSION=" + instConfig.Version,
		}

		for variable, value := range installOnlyVars {
			envs = append(envs, fmt.Sprintf("%s=%s", variable, value))
		}

		cmd := exec.CommandContext(app.JobsManager.Context(), installScriptPath)
		cmd.Dir = instanceDir
		cmd.Env = envs

		jobID, err := app.JobsManager.StartExec(ctx, instanceID, lockFilePath, jobType, cmd)
		if err != nil {
			return InstanceInstallResult{}, err
		}

		return InstanceInstallResult{
			JobID:        jobID,
			ID:           instanceID,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			AdminLogin:   "root",
			AdminPass:    "",
		}, nil
	}
}
