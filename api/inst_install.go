package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"eq/dcommsctl/internal/inst"
	"eq/dcommsctl/internal/jobs"
	"eq/dcommsctl/internal/proxy"
	"eq/dcommsctl/internal/tools"
)

type InstanceInstallArgs struct {
	AppName    string `json:"name"`
	DomainName string `json:"domain"`
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

func dawaCreateInstance(ctx context.Context, instancesDir string, config DawaConfig) (string, error) {
	// FIXME: this is a temporary measure, cause we ideally want to have multiple instances of dawa
	// or maybe we need another mechanism to upgrade dawa
	pathToApp := path.Join("conf_tmpl", "dawa")
	pathToInst := path.Join(instancesDir, "dawa")
	if installed, _ := dawaIsInstalled(instancesDir); installed {
		return pathToInst, nil
	}

	completed := false
	defer func() {
		if !completed {
			os.RemoveAll(pathToInst)
		}
	}()

	dirsToCreate := []string{
		"jobs",
	}

	for _, dir := range dirsToCreate {
		if err := os.MkdirAll(path.Join(pathToInst, dir), 0o700); err != nil {
			return "", fmt.Errorf("create dir %s for instance: %s", dir, err)
		}
	}

	if err := inst.WriteFileTemplate(
		path.Join(pathToApp, "docker-compose.tmpl.yml"),
		path.Join(pathToInst, "docker-compose.yml"),
		&config,
	); err != nil {
		return "", err
	}

	if err := inst.WriteFileTemplate(
		path.Join(pathToApp, "env.tmpl"),
		path.Join(pathToInst, ".env"),
		&config,
	); err != nil {
		return "", err
	}

	caddyRouteFilePath := path.Join(pathToInst, "caddy-route.json")
	if err := inst.WriteFileTemplate(
		path.Join(pathToApp, "caddy-route.tmpl.json"),
		caddyRouteFilePath,
		&config,
	); err != nil {
		return "", err
	}

	caddyRouteJSON, err := os.ReadFile(caddyRouteFilePath)
	if err != nil {
		return "", err
	}

	endpoint := "http://localhost:2019/config/apps/http/servers/srv0/routes"
	if _, _, err := proxy.CaddySendRequest(ctx, endpoint, http.MethodPost, "", caddyRouteJSON); err != nil {
		return "", err
	}

	if err := proxy.ExportConfig(ctx, instancesDir); err != nil {
		return "", err
	}

	completed = true

	return pathToInst, nil
}

func (app *GlobalState) InstanceInstall(ctx context.Context, args InstanceInstallArgs) (InstanceInstallResult, error) {
	applicationID := args.AppName
	jobType := "install"
	clientID, _ := randomID()
	clientSecret, _ := randomID()

	switch applicationID {

	case "caddy":
		{
			config := proxy.CaddyConfig{
				Domain: args.DomainName,
				Port:   8000,
			}
			instanceDir, err := proxy.InstanceSetup(app.SystemCfg.AppsDir, app.SystemCfg.InstancesDir, config)
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
	case "mastodon":
		{
			config := inst.InstanceGenericConfig{
				SSOIssuerURI: app.SystemCfg.SSODomain,
				Domain:       args.DomainName,
				Version:      "v4.5",

				Env: map[string]string{},
			}

			instanceDir, err := inst.InstanceSetup("mastodon", app.SystemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, "mastodon", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"VERSION=" + config.Version,
			}

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
	case "peertube":
		{
			config := inst.InstanceGenericConfig{
				SSOIssuerURI: app.SystemCfg.SSODomain,
				Domain:       args.DomainName,
				Env: map[string]string{
					"PORT_RMTP":   fmt.Sprintf("%d", tools.PortRandom()),
					"PORT_WEB_UI": fmt.Sprintf("%d", tools.PortRandom()),
				},
			}

			instanceDir, err := inst.InstanceSetup("peertube", app.SystemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, "peertube", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
				"OAUTH_CLIENT_ID=" + clientID,
				"OAUTH_CLIENT_SECRET=" + clientSecret,
			}

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
				AdminPass:    "test_pass",
			}, nil
		}
	case "cryptpad":
		{
			config := inst.InstanceGenericConfig{
				SSOIssuerURI: app.SystemCfg.SSODomain,
				Domain:       args.DomainName,
			}

			instanceDir, err := inst.InstanceSetup(applicationID, app.SystemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, applicationID, "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
				"OAUTH_CLIENT_ID=" + clientID,
				"OAUTH_CLIENT_SECRET=" + clientSecret,
			}

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
	case "synapse":
		{
			adminPass := "test_pass"
			config := inst.InstanceGenericConfig{
				SSOIssuerURI: app.SystemCfg.SSODomain,
				Domain:       args.DomainName,
			}

			instanceDir, err := inst.InstanceSetup(applicationID, app.SystemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, applicationID, "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
				"OAUTH_CLIENT_ID=" + clientID,
				"OAUTH_CLIENT_SECRET=" + clientSecret,
			}

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
				AdminPass:    adminPass,
			}, nil
		}
	case "ersatz":
		{
			config := inst.InstanceGenericConfig{
				Domain: args.DomainName,
			}

			instanceDir, err := inst.InstanceSetup(applicationID, app.SystemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, applicationID, "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
			}

			jobID, err := app.JobsManager.StartExec(ctx, instanceID, lockFilePath, jobType, cmd)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			return InstanceInstallResult{
				JobID:    jobID,
				ID:       instanceID,
				ClientID: clientID,
			}, nil
		}
	case "dawa":
		{
			config := DawaConfig{
				Domain:  args.DomainName,
				PortOut: 8081,
			}

			instanceDir, err := dawaCreateInstance(ctx, app.SystemCfg.InstancesDir, config)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			instanceID := filepath.Base(instanceDir)
			lockFilePath := path.Join(instanceDir, ".job.lock")

			if err := jobs.JobTryToLockFile(lockFilePath); err != nil {
				return InstanceInstallResult{}, fmt.Errorf("The job is in progress")
			}

			cmd := exec.CommandContext(app.JobsManager.Context(), path.Join(app.SystemCfg.AppsDir, "dawa", "install.sh"))
			cmd.Dir = instanceDir
			cmd.Env = []string{
				"APP_DIR=" + instanceDir,
			}

			jobID, err := app.JobsManager.StartExec(ctx, instanceID, lockFilePath, jobType, cmd)
			if err != nil {
				return InstanceInstallResult{}, err
			}

			// FIXME: update this in case of failure
			app.SystemCfg.SSODomain = args.DomainName

			return InstanceInstallResult{
				JobID:        jobID,
				ID:           instanceID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}, nil
		}
	}
	return InstanceInstallResult{}, fmt.Errorf("This application is not supported")
}
