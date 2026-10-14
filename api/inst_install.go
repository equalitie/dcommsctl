package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"eq/dcommsctl/internal/application"
	"eq/dcommsctl/internal/inst"
	"eq/dcommsctl/internal/jobs"
	"eq/dcommsctl/internal/proxy"
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

func randomID() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

type DawaConfig struct {
	Domain       string
	PortOut      int
	NginxDocRoot string
}

func dawaIsInstalled(instancesDir string) (bool, error) {
	pathToInst := path.Join(instancesDir, "dawa")
	if _, err := os.Stat(pathToInst); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (app *GlobalState) InstanceInstall(ctx context.Context, args InstanceInstallArgs) (InstanceInstallResult, error) {
	applicationName := args.AppName
	jobType := "install"
	clientID, _ := randomID()
	clientSecret, _ := randomID()

	switch applicationName {

	case "caddy":
		{
			instanceDir, err := proxy.InstanceSetup(app.SystemCfg.AppsDir, app.SystemCfg.InstancesDir, app.ProxyCfg)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			return InstanceInstallResult{
				JobID:        "",
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}, nil
		}
	case "ersatz", "peertube", "cryptpad", "mastodon", "synapse", "dawa", "metrics":
		{
			appDir := path.Join(app.SystemCfg.AppsDir, applicationName)

			appConfig, err := application.LoadConfig(path.Join(appDir, "config.json"))
			if err != nil {
				return InstanceInstallResult{}, fmt.Errorf("Failed to read config.json for the app %s: %s", applicationName, err)
			}

			envVars, installOnlyVars, err := application.ValidateParamsAgainstConfig(args.Variables, appConfig)
			if err != nil {
				return InstanceInstallResult{}, fmt.Errorf("Failed to validate variables for app %s", err)
			}

			instConfig := inst.InstanceGenericConfig{
				SSOIssuerURI: app.SystemCfg.SSODomain,
				Domain:       args.DomainName,
				Env:          envVars,
			}

			instanceDir, err := inst.InstanceSetup(applicationName, app.SystemCfg.InstancesDir, instConfig)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
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

			cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(appDir, "install.sh"))
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
	return InstanceInstallResult{}, fmt.Errorf("This application is not supported, %s", applicationName)
}
